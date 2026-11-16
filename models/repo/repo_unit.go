// Copyright 2017 The Gitea Authors. All rights reserved.
// SPDX-License-Identifier: MIT

package repo

import (
	"context"

	"gitea.dev/models/db"
	"gitea.dev/models/perm"
	"gitea.dev/models/unit"
	"gitea.dev/modules/json"
	"gitea.dev/modules/setting"
	"gitea.dev/modules/timeutil"
	"gitea.dev/modules/util"

	"xorm.io/xorm"
	"xorm.io/xorm/convert"
)

// ErrUnitTypeNotExist represents a "UnitTypeNotExist" kind of error.
type ErrUnitTypeNotExist struct {
	UT unit.Type
}

// IsErrUnitTypeNotExist checks if an error is a ErrUnitNotExist.
func IsErrUnitTypeNotExist(err error) bool {
	_, ok := err.(ErrUnitTypeNotExist)
	return ok
}

func (err ErrUnitTypeNotExist) Error() string {
	return "Unit type does not exist: " + err.UT.LogString()
}

func (err ErrUnitTypeNotExist) Unwrap() error {
	return util.ErrNotExist
}

// RepoUnit describes all units of a repository
type RepoUnit struct { //revive:disable-line:exported
	ID                  int64
	RepoID              int64              `xorm:"INDEX(s)"`
	Type                unit.Type          `xorm:"INDEX(s)"`
	Config              convert.Conversion `xorm:"TEXT"`
	CreatedUnix         timeutil.TimeStamp `xorm:"INDEX CREATED"`
	AnonymousAccessMode perm.AccessMode    `xorm:"NOT NULL DEFAULT 0"`
	EveryoneAccessMode  perm.AccessMode    `xorm:"NOT NULL DEFAULT 0"`
}

func init() {
	db.RegisterModel(new(RepoUnit))
}

// UnitConfig describes common unit config
type UnitConfig struct{}

// FromDB fills up a UnitConfig from serialized format.
func (cfg *UnitConfig) FromDB(bs []byte) error {
	return json.UnmarshalHandleDoubleEncode(bs, &cfg)
}

// ToDB exports a UnitConfig to a serialized format.
func (cfg *UnitConfig) ToDB() ([]byte, error) {
	return json.Marshal(cfg)
}

// PullRequestsConfig describes pull requests config
type PullRequestsConfig struct {
	IgnoreWhitespaceConflicts     bool
	AllowMerge                    bool
	AllowRebase                   bool
	AllowRebaseMerge              bool
	AllowSquash                   bool
	AllowFastForwardOnly          bool
	AllowManualMerge              bool
	AutodetectManualMerge         bool
	AllowMergeUpdate              bool
	AllowRebaseUpdate             bool
	DefaultUpdateStyle            UpdateStyle
	DefaultDeleteBranchAfterMerge bool
	DefaultMergeStyle             MergeStyle
	DefaultAllowMaintainerEdit    bool
	DefaultTargetBranch           string
}

func DefaultPullRequestsConfig() *PullRequestsConfig {
	cfg := &PullRequestsConfig{
		AllowMerge:                 true,
		AllowRebase:                true,
		AllowRebaseMerge:           true,
		AllowSquash:                true,
		AllowFastForwardOnly:       true,
		AllowMergeUpdate:           true,
		AllowRebaseUpdate:          true,
		DefaultUpdateStyle:         UpdateStyleMerge,
		DefaultAllowMaintainerEdit: true,
	}
	cfg.DefaultDeleteBranchAfterMerge = setting.Repository.PullRequest.DefaultDeleteBranchAfterMerge
	cfg.DefaultMergeStyle = MergeStyle(setting.Repository.PullRequest.DefaultMergeStyle)
	cfg.DefaultMergeStyle = util.IfZero(cfg.DefaultMergeStyle, MergeStyleMerge)
	return cfg
}

// FromDB fills up a PullRequestsConfig from serialized format.
func (cfg *PullRequestsConfig) FromDB(bs []byte) error {
	// set default values for existing PullRequestConfig in DB
	*cfg = *DefaultPullRequestsConfig()
	_ = json.UnmarshalHandleDoubleEncode(bs, &cfg) // don't let corrupted database value cause unnecessary 500 error
	cfg.DefaultUpdateStyle = util.IfZero(cfg.DefaultUpdateStyle, UpdateStyleMerge)
	return nil
}

// ToDB exports a PullRequestsConfig to a serialized format.
func (cfg *PullRequestsConfig) ToDB() ([]byte, error) {
	return json.Marshal(cfg)
}

// IsMergeStyleAllowed returns if merge style is allowed
func (cfg *PullRequestsConfig) IsMergeStyleAllowed(mergeStyle MergeStyle) bool {
	return mergeStyle == MergeStyleMerge && cfg.AllowMerge ||
		mergeStyle == MergeStyleRebase && cfg.AllowRebase ||
		mergeStyle == MergeStyleRebaseMerge && cfg.AllowRebaseMerge ||
		mergeStyle == MergeStyleSquash && cfg.AllowSquash ||
		mergeStyle == MergeStyleFastForwardOnly && cfg.AllowFastForwardOnly ||
		mergeStyle == MergeStyleManuallyMerged && cfg.AllowManualMerge
}

// IsUpdateStyleAllowed returns if a pull request branch update style is allowed
func (cfg *PullRequestsConfig) IsUpdateStyleAllowed(updateStyle UpdateStyle) bool {
	switch updateStyle {
	case UpdateStyleMerge:
		return cfg.AllowMergeUpdate
	case UpdateStyleRebase:
		return cfg.AllowRebaseUpdate
	default:
		return false
	}
}

// ValidateUpdateSettings checks that the AllowMerge/RebaseUpdate flags and DefaultUpdateStyle are mutually consistent.
func (cfg *PullRequestsConfig) ValidateUpdateSettings() error {
	if cfg.DefaultUpdateStyle != UpdateStyleMerge && cfg.DefaultUpdateStyle != UpdateStyleRebase {
		return util.NewInvalidArgumentErrorf("default update style must be merge or rebase")
	}
	if !cfg.AllowMergeUpdate && !cfg.AllowRebaseUpdate {
		return util.NewInvalidArgumentErrorf("at least one pull request branch update style must be enabled")
	}
	if !cfg.IsUpdateStyleAllowed(cfg.DefaultUpdateStyle) {
		return util.NewInvalidArgumentErrorf("default update style must be enabled")
	}
	return nil
}

func DefaultPullRequestsUnit(repoID int64) RepoUnit {
	return RepoUnit{RepoID: repoID, Type: unit.TypePullRequests, Config: DefaultPullRequestsConfig()}
}

// BeforeSet is invoked from XORM before setting the value of a field of this object.
func (r *RepoUnit) BeforeSet(colName string, val xorm.Cell) {
	switch colName {
	case "type":
		var err error
		r.Type, _, err = db.CellToInt(val, unit.TypeInvalid)
		if err != nil {
			setting.PanicInDevOrTesting("Unable to convert repo unit (id=%d) type: %v", r.ID, err)
		}
		switch r.Type {
		case unit.TypePullRequests:
			r.Config = new(PullRequestsConfig)
		default:
			r.Config = new(UnitConfig)
		}
	case "config":
		if *val == nil {
			// XROM doesn't call FromDB if the value is nil, but we need to set default values for the config fields
			_ = r.Config.FromDB(nil)
		}
	}
}

// Unit returns Unit
func (r *RepoUnit) Unit() unit.Unit {
	return unit.Units[r.Type]
}

// CodeConfig returns config for unit.TypeCode
func (r *RepoUnit) CodeConfig() *UnitConfig {
	return r.Config.(*UnitConfig)
}

// PullRequestsConfig returns config for unit.TypePullRequests
func (r *RepoUnit) PullRequestsConfig() *PullRequestsConfig {
	return r.Config.(*PullRequestsConfig)
}

// ReleasesConfig returns config for unit.TypeReleases
func (r *RepoUnit) ReleasesConfig() *UnitConfig {
	return r.Config.(*UnitConfig)
}

func getUnitsByRepoID(ctx context.Context, repoID int64) (units []*RepoUnit, err error) {
	var tmpUnits []*RepoUnit
	if err := db.GetEngine(ctx).Where("repo_id = ?", repoID).Find(&tmpUnits); err != nil {
		return nil, err
	}

	for _, u := range tmpUnits {
		if _, ok := unit.Units[u.Type]; ok && !u.Type.UnitGlobalDisabled() { // skip rows of removed unit types
			units = append(units, u)
		}
	}

	return units, nil
}

// UpdateRepoUnitConfig updates the config of the provided repo unit
func UpdateRepoUnitConfig(ctx context.Context, unit *RepoUnit) error {
	_, err := db.GetEngine(ctx).ID(unit.ID).Cols("config").Update(unit)
	return err
}

func UpdateRepoUnitPublicAccess(ctx context.Context, unit *RepoUnit) error {
	_, err := db.GetEngine(ctx).Where("repo_id=? AND `type`=?", unit.RepoID, unit.Type).
		Cols("anonymous_access_mode", "everyone_access_mode").Update(unit)
	return err
}
