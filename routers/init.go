// Copyright 2016 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package routers

import (
	"context"
	"net/http"
	"reflect"
	"runtime"
	"time"

	"gitea.dev/models"
	"gitea.dev/modules/cache"
	"gitea.dev/modules/git"
	"gitea.dev/modules/git/gitcmd"
	"gitea.dev/modules/gitrepo"
	"gitea.dev/modules/log"
	"gitea.dev/modules/queue"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/storage"
	"gitea.dev/modules/system"
	"gitea.dev/modules/util"
	"gitea.dev/modules/validation"
	"gitea.dev/modules/web"
	"gitea.dev/modules/web/routing"
	apiv1 "gitea.dev/routers/api/v1"
	"gitea.dev/routers/common"
	"gitea.dev/routers/private"
	web_routers "gitea.dev/routers/web"
	pull_service "gitea.dev/services/pull"
	release_service "gitea.dev/services/release"
	repo_service "gitea.dev/services/repository"
	"gitea.dev/services/webhook"
)

func mustInit(fn func() error) {
	err := fn()
	if err != nil {
		ptr := reflect.ValueOf(fn).Pointer()
		fi := runtime.FuncForPC(ptr)
		log.Fatal("%s failed: %v", fi.Name(), err)
	}
}

func mustInitCtx(ctx context.Context, fn func(ctx context.Context) error) {
	err := fn(ctx)
	if err != nil {
		ptr := reflect.ValueOf(fn).Pointer()
		fi := runtime.FuncForPC(ptr)
		log.Fatal("%s(ctx) failed: %v", fi.Name(), err)
	}
}

func syncAppConfForGit(ctx context.Context) error {
	runtimeState := new(system.RuntimeState)
	if err := system.AppState.Get(ctx, runtimeState); err != nil {
		return err
	}

	updated := false
	if runtimeState.LastAppPath != setting.AppPath {
		log.Info("AppPath changed from '%s' to '%s'", runtimeState.LastAppPath, setting.AppPath)
		runtimeState.LastAppPath = setting.AppPath
		updated = true
	}
	if runtimeState.LastCustomConf != setting.CustomConf {
		log.Info("CustomConf changed from '%s' to '%s'", runtimeState.LastCustomConf, setting.CustomConf)
		runtimeState.LastCustomConf = setting.CustomConf
		updated = true
	}

	if hooksHash := gitrepo.HookTemplatesHash(); runtimeState.LastHooksHash != hooksHash {
		log.Info("repository hook templates changed")
		runtimeState.LastHooksHash = hooksHash
		updated = true
	}

	if updated {
		log.Info("re-sync repository hooks ...")
		mustInitCtx(ctx, repo_service.SyncRepositoryHooks)

		return system.AppState.Set(ctx, runtimeState)
	}
	return nil
}

// InitWebInstalled is for the global configuration of an installed instance
func InitWebInstalled(ctx context.Context) {
	// git init runs two git processes and the queue database takes a while to open, so both overlap
	// with the settings and the ORM engine, which use neither; everything after them may use git
	gitInitErr := make(chan error, 1)
	go func() { gitInitErr <- git.InitFull() }()

	// Setup i18n

	setting.LoadSettings()
	releaseLevelDB := queue.OpenLevelDBEarly()
	mustInit(storage.Init)

	mustInit(cache.Init)

	mustInitCtx(ctx, common.InitDBEngine)
	log.Info("ORM engine initialization successful!")

	if err := <-gitInitErr; err != nil {
		log.Fatal("git.InitFull failed: %v", err)
	}
	git.EnableBatchPool(ctx, 30*time.Second, setting.RepoRootPath)
	log.Info("Git version: %s (home: %s)", git.DefaultFeatures().VersionInfo(), gitcmd.HomeDir())
	if !git.DefaultFeatures().SupportHashSha256 {
		log.Warn("sha256 hash support is disabled - requires Git >= 2.42." + util.Iif(git.DefaultFeatures().UsingGogit, " Gogit is currently unsupported.", ""))
	}

	mustInit(system.Init)
	mustInit(release_service.Init)

	mustInitCtx(ctx, models.Init)
	mustInitCtx(ctx, repo_service.Init)

	// Booting long running goroutines.

	mustInit(webhook.Init)
	mustInit(pull_service.Init)

	mustInitCtx(ctx, syncAppConfForGit)
	releaseLevelDB()
}

// NormalRoutes represents non install routes
func NormalRoutes() *web.Router {
	validation.AddBindingRules() // custom binding rules used by API option structs, e.g. GitRefName

	r := web.NewRouter()
	r.BeforeRouting(common.ProtocolMiddlewares()...)

	r.Mount("/", web_routers.Routes())
	r.Mount("/api/v1", apiv1.Routes())
	r.Mount("/api/internal", private.Routes())

	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		defer routing.RecordFuncInfo(req.Context(), routing.GetFuncInfo(http.NotFound, "GlobalNotFound"))()
		http.NotFound(w, req)
	})
	return r
}
