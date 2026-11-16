// Copyright 2018 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package access

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"gitea.dev/models/db"
	perm_model "gitea.dev/models/perm"
	repo_model "gitea.dev/models/repo"
	"gitea.dev/models/unit"
	user_model "gitea.dev/models/user"
	"gitea.dev/modules/container"
	"gitea.dev/modules/log"
	"gitea.dev/modules/util"
)

// Permission contains all the permissions related variables to a repository for a user
type Permission struct {
	AccessMode perm_model.AccessMode

	units     []*repo_model.RepoUnit
	unitsMode map[unit.Type]perm_model.AccessMode
}

// IsOwner returns true if current user is the owner of repository.
func (p *Permission) IsOwner() bool {
	return p.AccessMode >= perm_model.AccessModeOwner
}

// IsAdmin returns true if current user has admin or higher access of repository.
func (p *Permission) IsAdmin() bool {
	return p.AccessMode >= perm_model.AccessModeAdmin
}

// HasAnyUnitAccess returns true if the user has at least read access to any unit of this repository
func (p *Permission) HasAnyUnitAccess() bool {
	for _, v := range p.unitsMode {
		if v >= perm_model.AccessModeRead {
			return true
		}
	}
	return p.AccessMode >= perm_model.AccessModeRead
}

// HasUnits returns true if the permission contains attached units
func (p *Permission) HasUnits() bool {
	return len(p.units) > 0
}

// GetFirstUnitRepoID returns the repo ID of the first unit, it is a fragile design and should NOT be used anymore
// deprecated
func (p *Permission) GetFirstUnitRepoID() int64 {
	if len(p.units) > 0 {
		return p.units[0].RepoID
	}
	return 0
}

// UnitAccessMode returns current user access mode to the specify unit of the repository
func (p *Permission) UnitAccessMode(unitType unit.Type) perm_model.AccessMode {
	// if the units map contains the access mode, use it, but admin/owner mode could override it
	if m, ok := p.unitsMode[unitType]; ok {
		return util.Iif(p.AccessMode >= perm_model.AccessModeAdmin, p.AccessMode, m)
	}
	// if the units map does not contain the access mode, return the default access mode if the unit exists
	hasUnit := slices.ContainsFunc(p.units, func(u *repo_model.RepoUnit) bool { return u.Type == unitType })
	return util.Iif(hasUnit, p.AccessMode, perm_model.AccessModeNone)
}

func (p *Permission) SetUnitsWithDefaultAccessMode(units []*repo_model.RepoUnit, mode perm_model.AccessMode) {
	p.units = units
	p.unitsMode = make(map[unit.Type]perm_model.AccessMode)
	for _, u := range p.units {
		p.unitsMode[u.Type] = mode
	}
}

// CanAccess returns true if user has mode access to the unit of the repository
func (p *Permission) CanAccess(mode perm_model.AccessMode, unitType unit.Type) bool {
	return p.UnitAccessMode(unitType) >= mode
}

// CanAccessAny returns true if user has mode access to any of the units of the repository
func (p *Permission) CanAccessAny(mode perm_model.AccessMode, unitTypes ...unit.Type) bool {
	for _, u := range unitTypes {
		if p.CanAccess(mode, u) {
			return true
		}
	}
	return false
}

// CanRead returns true if user could read to this unit
func (p *Permission) CanRead(unitType unit.Type) bool {
	return p.CanAccess(perm_model.AccessModeRead, unitType)
}

// CanReadAny returns true if user has read access to any of the units of the repository
func (p *Permission) CanReadAny(unitTypes ...unit.Type) bool {
	return p.CanAccessAny(perm_model.AccessModeRead, unitTypes...)
}

// CanReadIssuesOrPulls returns true if isPull is true and user could read pull requests and
// returns true if isPull is false and user could read to issues
func (p *Permission) CanReadIssuesOrPulls(isPull bool) bool {
	if isPull {
		return p.CanRead(unit.TypePullRequests)
	}
	return p.CanRead(unit.TypeIssues)
}

// CanWrite returns true if user could write to this unit
func (p *Permission) CanWrite(unitType unit.Type) bool {
	return p.CanAccess(perm_model.AccessModeWrite, unitType)
}

// CanWriteIssuesOrPulls returns true if isPull is true and user could write to pull requests and
// returns true if isPull is false and user could write to issues
func (p *Permission) CanWriteIssuesOrPulls(isPull bool) bool {
	if isPull {
		return p.CanWrite(unit.TypePullRequests)
	}
	return p.CanWrite(unit.TypeIssues)
}

func (p *Permission) ReadableUnitTypes() []unit.Type {
	types := make([]unit.Type, 0, len(p.units))
	for _, u := range p.units {
		if p.CanRead(u.Type) {
			types = append(types, u.Type)
		}
	}
	return types
}

func (p *Permission) LogString() string {
	var format strings.Builder
	format.WriteString("<Permission AccessMode=%s, %d Units, %d UnitsMode(s): [")
	args := []any{p.AccessMode.ToString(), len(p.units), len(p.unitsMode)}

	for i, u := range p.units {
		config := ""
		if u.Config != nil {
			configBytes, err := u.Config.ToDB()
			config = string(configBytes)
			if err != nil {
				config = err.Error()
			}
		}
		format.WriteString("\n\tunits[%d]: ID=%d RepoID=%d Type=%s Config=%s")
		args = append(args, i, u.ID, u.RepoID, u.Type.LogString(), config)
	}
	for key, value := range p.unitsMode {
		format.WriteString("\n\tunitsMode[%-v]: %-v")
		args = append(args, key.LogString(), value.LogString())
	}
	format.WriteString("\n\t]>")
	return fmt.Sprintf(format.String(), args...)
}

// GetDoerRepoPermission returns the repository permission for the current actor
func GetDoerRepoPermission(ctx context.Context, repo *repo_model.Repository, user *user_model.User) (Permission, error) {
	return GetIndividualUserRepoPermission(ctx, repo, user)
}

// GetIndividualUserRepoPermission returns the permissions for an explicit user identity.
// The server has a single owner: any signed-in user has full access to every repository,
// anonymous requests have none. In most request paths, callers should use GetDoerRepoPermission instead.
func GetIndividualUserRepoPermission(ctx context.Context, repo *repo_model.Repository, user *user_model.User) (perm Permission, err error) {
	if err = repo.LoadUnits(ctx); err != nil {
		return perm, err
	}
	perm.units = repo.Units
	if user != nil && user.ID > 0 {
		perm.AccessMode = perm_model.AccessModeOwner
	}
	log.Trace("Permission Loaded for user %-v in repo %-v, permissions: %-+v", user, repo, perm)
	return perm, nil
}

// IsUserRepoAdmin return true if user has admin right of a repo
func IsUserRepoAdmin(_ context.Context, repo *repo_model.Repository, user *user_model.User) (bool, error) {
	return user != nil && user.ID > 0 && repo != nil, nil
}

// AccessLevel returns the Access a user has to a repository. Will return NoneAccess if the
// user does not have access.
func AccessLevel(ctx context.Context, user *user_model.User, repo *repo_model.Repository) (perm_model.AccessMode, error) { //nolint:revive // export stutter
	return AccessLevelUnit(ctx, user, repo, unit.TypeCode)
}

// AccessLevelUnit returns the Access a user has to a repository's. Will return NoneAccess if the
// user does not have access.
func AccessLevelUnit(ctx context.Context, user *user_model.User, repo *repo_model.Repository, unitType unit.Type) (perm_model.AccessMode, error) { //nolint:revive // export stutter
	perm, err := GetIndividualUserRepoPermission(ctx, repo, user)
	if err != nil {
		return perm_model.AccessModeNone, err
	}
	return perm.UnitAccessMode(unitType), nil
}

// HasAccessUnit returns true if user has testMode to the unit of the repository
func HasAccessUnit(ctx context.Context, user *user_model.User, repo *repo_model.Repository, unitType unit.Type, testMode perm_model.AccessMode) (bool, error) {
	mode, err := AccessLevelUnit(ctx, user, repo, unitType)
	return testMode <= mode, err
}

// CanBeAssigned return true if user can be assigned to issue or pull requests in repo
// Currently any write access (code, issues or pr's) is assignable, to match assignee list in user interface.
func CanBeAssigned(ctx context.Context, user *user_model.User, repo *repo_model.Repository) (bool, error) {
	if user.IsOrganization() {
		return false, util.NewInvalidArgumentErrorf("organization can't be added as assignee [user_id: %d, repo_id: %d]", user.ID, repo.ID)
	}
	perm, err := GetIndividualUserRepoPermission(ctx, repo, user)
	if err != nil {
		return false, err
	}
	return perm.CanAccessAny(perm_model.AccessModeWrite, unit.AllRepoUnitTypes...) ||
		perm.CanAccessAny(perm_model.AccessModeRead, unit.TypePullRequests), nil
}

// HasAnyUnitAccess see the comment of "perm.HasAnyUnitAccess"
func HasAnyUnitAccess(ctx context.Context, userID int64, repo *repo_model.Repository) (bool, error) {
	var user *user_model.User
	var err error
	if userID > 0 {
		user, err = user_model.GetUserByID(ctx, userID)
		if err != nil {
			return false, err
		}
	}
	perm, err := GetIndividualUserRepoPermission(ctx, repo, user)
	if err != nil {
		return false, err
	}
	return perm.HasAnyUnitAccess(), nil
}

// GetUsersWithAnyUnitAccess returns the users with access to the repository, which is every individual user
func GetUsersWithAnyUnitAccess(ctx context.Context, repo *repo_model.Repository, _ perm_model.AccessMode, _ unit.Type, _ ...unit.Type) (users []*user_model.User, err error) {
	if err = repo.LoadUnits(ctx); err != nil {
		return nil, err
	}
	if len(repo.Units) == 0 {
		return users, nil
	}
	err = db.GetEngine(ctx).Where("`type` = ?", user_model.UserTypeIndividual).OrderBy("`name`").Find(&users)
	return users, err
}

// GetUserIDsWithAnyUnitAccess returns the IDs of the users with access to the repository, see GetUsersWithAnyUnitAccess
func GetUserIDsWithAnyUnitAccess(ctx context.Context, repo *repo_model.Repository, mode perm_model.AccessMode, unitType unit.Type, moreUnitTypes ...unit.Type) (container.Set[int64], error) {
	users, err := GetUsersWithAnyUnitAccess(ctx, repo, mode, unitType, moreUnitTypes...)
	if err != nil {
		return nil, err
	}
	userIDs := make(container.Set[int64], len(users))
	for _, u := range users {
		userIDs.Add(u.ID)
	}
	return userIDs, nil
}
