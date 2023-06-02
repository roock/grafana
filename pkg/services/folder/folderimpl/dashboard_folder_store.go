package folderimpl

import (
	"context"
	"fmt"
	"time"

	"github.com/grafana/grafana/pkg/infra/db"
	"github.com/grafana/grafana/pkg/infra/localcache"
	"github.com/grafana/grafana/pkg/services/dashboards"
	"github.com/grafana/grafana/pkg/services/folder"
)

const (
	CACHING_FOLDER_BY_UID_PREFIX   = "folderByUID"
	CACHING_FOLDER_BY_TITLE_PREFIX = "folderByTitle"
	CACHING_FOLDER_BY_ID_PREFIX    = "folderByID"
)

// DashboardStore implements the FolderStore interface
// It fetches folders from the dashboard DB table
type DashboardFolderStoreImpl struct {
	store        db.DB
	caching      bool
	cacheService *localcache.CacheService
}

func ProvideDashboardFolderStore(sqlStore db.DB, cacheService *localcache.CacheService) *DashboardFolderStoreImpl {
	return &DashboardFolderStoreImpl{store: sqlStore, cacheService: cacheService, caching: true}
}

func (d *DashboardFolderStoreImpl) DisableCaching() {
	d.caching = false
}

func (d *DashboardFolderStoreImpl) GetFolderByTitle(ctx context.Context, orgID int64, title string) (*folder.Folder, error) {
	if title == "" {
		return nil, dashboards.ErrFolderTitleEmpty
	}

	cacheKey := fmt.Sprintf("%s-%d-%s", CACHING_FOLDER_BY_TITLE_PREFIX, orgID, title)
	return d.withCaching(cacheKey, func() (*folder.Folder, error) {
		// there is a unique constraint on org_id, folder_id, title
		// there are no nested folders so the parent folder id is always 0
		dashboard := dashboards.Dashboard{OrgID: orgID, FolderID: 0, Title: title}
		err := d.store.WithTransactionalDbSession(ctx, func(sess *db.Session) error {
			has, err := sess.Table(&dashboards.Dashboard{}).Where("is_folder = " + d.store.GetDialect().BooleanStr(true)).Where("folder_id=0").Get(&dashboard)
			if err != nil {
				return err
			}
			if !has {
				return dashboards.ErrFolderNotFound
			}
			dashboard.SetID(dashboard.ID)
			dashboard.SetUID(dashboard.UID)
			return nil
		})
		return dashboards.FromDashboard(&dashboard), err
	})
}

func (d *DashboardFolderStoreImpl) GetFolderByID(ctx context.Context, orgID int64, id int64) (*folder.Folder, error) {
	cacheKey := fmt.Sprintf("%s-%d-%d", CACHING_FOLDER_BY_ID_PREFIX, orgID, id)
	return d.withCaching(cacheKey, func() (*folder.Folder, error) {
		dashboard := dashboards.Dashboard{OrgID: orgID, FolderID: 0, ID: id}
		err := d.store.WithTransactionalDbSession(ctx, func(sess *db.Session) error {
			has, err := sess.Table(&dashboards.Dashboard{}).Where("is_folder = " + d.store.GetDialect().BooleanStr(true)).Where("folder_id=0").Get(&dashboard)
			if err != nil {
				return err
			}
			if !has {
				return dashboards.ErrFolderNotFound
			}
			dashboard.SetID(dashboard.ID)
			dashboard.SetUID(dashboard.UID)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return dashboards.FromDashboard(&dashboard), nil
	})
}

func (d *DashboardFolderStoreImpl) GetFolderByUID(ctx context.Context, orgID int64, uid string) (*folder.Folder, error) {
	if uid == "" {
		return nil, dashboards.ErrDashboardIdentifierNotSet
	}

	cacheKey := fmt.Sprintf("%s-%d-%s", CACHING_FOLDER_BY_UID_PREFIX, orgID, uid)
	return d.withCaching(cacheKey, func() (*folder.Folder, error) {
		dashboard := dashboards.Dashboard{OrgID: orgID, FolderID: 0, UID: uid}
		err := d.store.WithTransactionalDbSession(ctx, func(sess *db.Session) error {
			has, err := sess.Table(&dashboards.Dashboard{}).Where("is_folder = " + d.store.GetDialect().BooleanStr(true)).Where("folder_id=0").Get(&dashboard)
			if err != nil {
				return err
			}
			if !has {
				return dashboards.ErrFolderNotFound
			}
			dashboard.SetID(dashboard.ID)
			dashboard.SetUID(dashboard.UID)
			return nil
		})
		if err != nil {
			return nil, err
		}

		res := dashboards.FromDashboard(&dashboard)
		return res, nil
	})
}

func (d *DashboardFolderStoreImpl) withCaching(cacheKey string, f func() (*folder.Folder, error)) (*folder.Folder, error) {
	if !d.caching {
		return f()
	}

	if f, ok := d.cacheService.Get(cacheKey); ok {
		return f.(*folder.Folder), nil
	}

	res, err := f()
	if err != nil {
		return nil, err
	}

	d.cacheService.Set(cacheKey, res, time.Second*5)
	return res, nil
}
