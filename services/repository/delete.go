// Copyright 2023 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repository

import (
	"context"
	"fmt"

	activities_model "gitea.dev/models/activities"
	admin_model "gitea.dev/models/admin"
	"gitea.dev/models/db"
	git_model "gitea.dev/models/git"
	issues_model "gitea.dev/models/issues"
	"gitea.dev/models/organization"
	access_model "gitea.dev/models/perm/access"
	project_model "gitea.dev/models/project"
	repo_model "gitea.dev/models/repo"
	system_model "gitea.dev/models/system"
	user_model "gitea.dev/models/user"
	"gitea.dev/models/webhook"
	"gitea.dev/modules/gitrepo"
	"gitea.dev/modules/graceful"
	"gitea.dev/modules/lfs"
	"gitea.dev/modules/log"
	"gitea.dev/modules/storage"
	asymkey_service "gitea.dev/services/asymkey"
	issue_service "gitea.dev/services/issue"

	"xorm.io/builder"
)

func deleteDBRepository(ctx context.Context, repoID int64) error {
	if cnt, err := db.GetEngine(ctx).ID(repoID).Delete(&repo_model.Repository{}); err != nil {
		return err
	} else if cnt != 1 {
		return repo_model.ErrRepoNotExist{
			ID:        repoID,
			OwnerName: "",
			Name:      "",
		}
	}
	return nil
}

// DeleteRepository deletes a repository for a user or organization.
// make sure if you call this func to close open sessions (sqlite will otherwise get a deadlock)
func DeleteRepositoryDirectly(ctx context.Context, repoID int64, ignoreOrgTeams ...bool) error {
	ctx, committer, err := db.TxContext(ctx)
	if err != nil {
		return err
	}
	defer committer.Close()
	sess := db.GetEngine(ctx)

	repo := &repo_model.Repository{}
	has, err := sess.ID(repoID).Get(repo)
	if err != nil {
		return err
	} else if !has {
		return repo_model.ErrRepoNotExist{
			ID:        repoID,
			OwnerName: "",
			Name:      "",
		}
	}

	// In case owner is a organization, we have to change repo specific teams
	// if ignoreOrgTeams is not true
	var org *user_model.User
	if len(ignoreOrgTeams) == 0 || !ignoreOrgTeams[0] {
		if org, err = user_model.GetUserByID(ctx, repo.OwnerID); err != nil {
			return err
		}
	}

	// Delete Deploy Keys
	deleted, err := asymkey_service.DeleteRepoDeployKeys(ctx, repoID)
	if err != nil {
		return err
	}
	needRewriteKeysFile := deleted > 0

	if err := deleteDBRepository(ctx, repoID); err != nil {
		return err
	}

	if org != nil && org.IsOrganization() {
		teams, err := organization.FindOrgTeams(ctx, org.ID)
		if err != nil {
			return err
		}
		for _, t := range teams {
			if !organization.HasTeamRepo(ctx, t.OrgID, t.ID, repoID) {
				continue
			} else if err = removeRepositoryFromTeam(ctx, t, repo, false); err != nil {
				return err
			}
		}
	}

	attachments := make([]*repo_model.Attachment, 0, 20)
	if err = sess.Join("INNER", "`release`", "`release`.id = `attachment`.release_id").
		Where("`release`.repo_id = ?", repoID).
		Find(&attachments); err != nil {
		return err
	}
	releaseAttachments := make([]string, 0, len(attachments))
	for i := 0; i < len(attachments); i++ {
		releaseAttachments = append(releaseAttachments, attachments[i].RelativePath())
	}

	if _, err := db.Exec(ctx, "UPDATE `user` SET num_stars=num_stars-1 WHERE id IN (SELECT `uid` FROM `star` WHERE repo_id = ?)", repo.ID); err != nil {
		return err
	}

	if _, err := db.GetEngine(ctx).In("hook_id", builder.Select("id").From("webhook").Where(builder.Eq{"webhook.repo_id": repo.ID})).
		Delete(&webhook.HookTask{}); err != nil {
		return err
	}

	if err := db.DeleteBeans(ctx,
		&access_model.Access{RepoID: repo.ID},
		&activities_model.Action{RepoID: repo.ID},
		&repo_model.Collaboration{RepoID: repoID},
		&issues_model.Comment{RefRepoID: repoID},
		&git_model.CommitStatus{RepoID: repoID},
		&git_model.CommitStatusIndex{RepoID: repoID},
		&git_model.CommitStatusSummary{RepoID: repoID},
		&git_model.Branch{RepoID: repoID},
		&git_model.RenamedBranch{RepoID: repoID},
		&git_model.LFSLock{RepoID: repoID},
		&repo_model.LanguageStat{RepoID: repoID},
		&repo_model.RepoLicense{RepoID: repoID},
		&issues_model.Milestone{RepoID: repoID},
		&repo_model.Mirror{RepoID: repoID},
		&activities_model.Notification{RepoID: repoID},
		&git_model.ProtectedBranch{RepoID: repoID},
		&git_model.ProtectedTag{RepoID: repoID},
		&repo_model.PushMirror{RepoID: repoID},
		&repo_model.Release{RepoID: repoID},
		&repo_model.RepoIndexerStatus{RepoID: repoID},
		&repo_model.Redirect{RedirectRepoID: repoID},
		&repo_model.RepoTransfer{RepoID: repoID}, // this column doesn't have index, maybe it's fine since the table shouldn't be too large.
		&repo_model.RepoUnit{RepoID: repoID},
		&repo_model.Star{RepoID: repoID},
		&admin_model.Task{RepoID: repoID},
		&repo_model.Watch{RepoID: repoID},
		&webhook.Webhook{RepoID: repoID},
		&issues_model.IssuePin{RepoID: repoID},
	); err != nil {
		return fmt.Errorf("deleteBeans: %w", err)
	}

	// Delete Labels and related objects
	if err := issues_model.DeleteLabelsByRepoID(ctx, repoID); err != nil {
		return err
	}

	// Delete Pulls and related objects
	if err := issues_model.DeletePullsByBaseRepoID(ctx, repoID); err != nil {
		return err
	}

	// Delete Issues and related objects
	var attachmentPaths []string
	if attachmentPaths, err = issue_service.DeleteIssuesByRepoID(ctx, repoID); err != nil {
		return err
	}

	// Delete issue index
	if err := db.DeleteResourceIndex(ctx, "issue_index", repoID); err != nil {
		return err
	}

	if repo.IsFork {
		if _, err := db.Exec(ctx, "UPDATE `repository` SET num_forks=num_forks-1 WHERE id=?", repo.ForkID); err != nil {
			return fmt.Errorf("decrease fork count: %w", err)
		}
	}

	if _, err := db.Exec(ctx, "UPDATE `user` SET num_repos=num_repos-1 WHERE id=?", repo.OwnerID); err != nil {
		return err
	}

	if len(repo.Topics) > 0 {
		if err := repo_model.RemoveTopicsFromRepo(ctx, repo.ID); err != nil {
			return err
		}
	}

	if err := project_model.DeleteProjectByRepoID(ctx, repoID); err != nil {
		return fmt.Errorf("unable to delete projects for repo[%d]: %w", repoID, err)
	}

	// Remove LFS objects
	var lfsObjects []*git_model.LFSMetaObject
	if err = sess.Where("repository_id=?", repoID).Find(&lfsObjects); err != nil {
		return err
	}

	lfsPaths := make([]string, 0, len(lfsObjects))
	for _, v := range lfsObjects {
		count, err := db.CountByBean(ctx, &git_model.LFSMetaObject{Pointer: lfs.Pointer{Oid: v.Oid}})
		if err != nil {
			return err
		}
		if count > 1 {
			continue
		}

		lfsPaths = append(lfsPaths, v.RelativePath())
	}

	if _, err := db.DeleteByBean(ctx, &git_model.LFSMetaObject{RepositoryID: repoID}); err != nil {
		return err
	}

	// Remove archives
	var archives []*repo_model.RepoArchiver
	if err = sess.Where("repo_id=?", repoID).Find(&archives); err != nil {
		return err
	}

	archivePaths := make([]string, 0, len(archives))
	for _, v := range archives {
		archivePaths = append(archivePaths, v.RelativePath())
	}

	if _, err := db.DeleteByBean(ctx, &repo_model.RepoArchiver{RepoID: repoID}); err != nil {
		return err
	}

	if repo.NumForks > 0 {
		if _, err = sess.Exec("UPDATE `repository` SET fork_id=0,is_fork=? WHERE fork_id=?", false, repo.ID); err != nil {
			log.Error("reset 'fork_id' and 'is_fork': %v", err)
		}
	}

	// Get all attachments with both issue_id and release_id are zero
	var newAttachments []*repo_model.Attachment
	if err := sess.Where(builder.Eq{
		"repo_id":    repo.ID,
		"issue_id":   0,
		"release_id": 0,
	}).Find(&newAttachments); err != nil {
		return err
	}

	newAttachmentPaths := make([]string, 0, len(newAttachments))
	for _, attach := range newAttachments {
		newAttachmentPaths = append(newAttachmentPaths, attach.RelativePath())
	}

	if _, err := sess.Where("repo_id=?", repo.ID).Delete(new(repo_model.Attachment)); err != nil {
		return err
	}

	if err = committer.Commit(); err != nil {
		return err
	}

	committer.Close()

	if needRewriteKeysFile {
		if err := asymkey_service.RewriteAllPublicKeys(ctx); err != nil {
			log.Error("RewriteAllPublicKeys failed: %v", err)
		}
	}

	// We should always delete the files after the database transaction succeed. If
	// we delete the file but the database rollback, the repository will be broken.

	// Remove repository files.
	if err := gitrepo.DeleteRepository(ctx, repo); err != nil {
		desc := fmt.Sprintf("Delete repository files (%s): %v", repo.FullName(), err)
		if err = system_model.CreateNotice(graceful.GetManager().ShutdownContext(), system_model.NoticeRepository, desc); err != nil {
			log.Error("CreateRepositoryNotice: %v", err)
		}
	}

	// Remove wiki files if it exists.
	if err := gitrepo.DeleteRepository(ctx, repo.WikiStorageRepo()); err != nil {
		desc := fmt.Sprintf("Delete wiki repository files (%s): %v", repo.FullName(), err)
		// Note we use the db.DefaultContext here rather than passing in a context as the context may be cancelled
		if err = system_model.CreateNotice(graceful.GetManager().ShutdownContext(), system_model.NoticeRepository, desc); err != nil {
			log.Error("CreateRepositoryNotice: %v", err)
		}
	}

	// Remove archives
	for _, archive := range archivePaths {
		system_model.RemoveStorageWithNotice(ctx, storage.RepoArchives, "Delete repo archive file", archive)
	}

	// Remove lfs objects
	for _, lfsObj := range lfsPaths {
		system_model.RemoveStorageWithNotice(ctx, storage.LFS, "Delete orphaned LFS file", lfsObj)
	}

	// Remove issue attachment files.
	for _, attachment := range attachmentPaths {
		system_model.RemoveStorageWithNotice(ctx, storage.Attachments, "Delete issue attachment", attachment)
	}

	// Remove release attachment files.
	for _, releaseAttachment := range releaseAttachments {
		system_model.RemoveStorageWithNotice(ctx, storage.Attachments, "Delete release attachment", releaseAttachment)
	}

	// Remove attachment with no issue_id and release_id.
	for _, newAttachment := range newAttachmentPaths {
		system_model.RemoveStorageWithNotice(ctx, storage.Attachments, "Delete issue attachment", newAttachment)
	}

	if len(repo.Avatar) > 0 {
		if err := storage.RepoAvatars.Delete(repo.CustomAvatarRelativePath()); err != nil {
			log.Error("remove avatar file %q: %v", repo.CustomAvatarRelativePath(), err)
			// go on
		}
	}

	return nil
}

// DeleteOwnerRepositoriesDirectly calls DeleteRepositoryDirectly for all repos of the given owner
func DeleteOwnerRepositoriesDirectly(ctx context.Context, owner *user_model.User) error {
	for {
		repos, _, err := repo_model.GetUserRepositories(ctx, repo_model.SearchRepoOptions{
			ListOptions: db.ListOptions{
				PageSize: repo_model.RepositoryListDefaultPageSize,
				Page:     1,
			},
			Private: true,
			OwnerID: owner.ID,
			Actor:   owner,
		})
		if err != nil {
			return fmt.Errorf("GetUserRepositories: %w", err)
		}
		if len(repos) == 0 {
			break
		}
		for _, repo := range repos {
			if err := DeleteRepositoryDirectly(ctx, repo.ID); err != nil {
				return fmt.Errorf("unable to delete repository %s for %s[%d]. Error: %w", repo.Name, owner.Name, owner.ID, err)
			}
		}
	}
	return nil
}
