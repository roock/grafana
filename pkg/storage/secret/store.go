package secret

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/grafana/authlib/claims"
	"github.com/grafana/grafana/pkg/apimachinery/utils"
	secret "github.com/grafana/grafana/pkg/apis/secret/v0alpha1"
	"github.com/grafana/grafana/pkg/infra/db"
	"github.com/grafana/grafana/pkg/services/sqlstore/session"
	"github.com/grafana/grafana/pkg/setting"
	"github.com/grafana/grafana/pkg/storage/unified/sql/sqltemplate"
	"github.com/grafana/grafana/pkg/util"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
)

type SecureValueStore interface {
	Create(ctx context.Context, s *secret.SecureValue) (*secret.SecureValue, error)
	Update(ctx context.Context, s *secret.SecureValue) (*secret.SecureValue, error)
	Delete(ctx context.Context, ns string, name string) (*secret.SecureValue, bool, error)
	List(ctx context.Context, ns string, options *internalversion.ListOptions) (*secret.SecureValueList, error)

	// The value will not be included
	Read(ctx context.Context, ns string, name string) (*secret.SecureValue, error)

	// Return a version that has the secure value visible
	Decrypt(ctx context.Context, ns string, name string) (*secret.SecureValue, error)

	// Show the history for a single value
	History(ctx context.Context, ns string, name string, continueToken string) (*secret.SecureValueActivityList, error)
}

func ProvideSecureValueStore(db db.DB, keeper SecretKeeper, cfg *setting.Cfg) (SecureValueStore, error) {
	err := MigrateSecretStore(context.Background(), db.GetEngine(), cfg)
	if err != nil {
		return nil, err
	}

	// One version of DB?
	return &secureStore{
		keeper:  keeper,
		db:      db.GetSqlxSession(),
		dialect: sqltemplate.DialectForDriver(string(db.GetDBType())),
		authz:   &authorizer{},
	}, nil
}

func CleanAnnotations(anno map[string]string) map[string]string {
	copy := make(map[string]string)
	for k, v := range anno {
		if skipAnnotations[k] {
			continue
		}
		copy[k] = v
	}
	return copy
}

var (
	_ SecureValueStore = (*secureStore)(nil)

	// Exclude these annotations
	skipAnnotations = map[string]bool{
		"kubectl.kubernetes.io/last-applied-configuration": true, // force server side apply
		utils.AnnoKeyCreatedBy:                             true,
		utils.AnnoKeyUpdatedBy:                             true,
		utils.AnnoKeyUpdatedTimestamp:                      true,
	}
)

type secureStore struct {
	keeper  SecretKeeper
	db      *session.SessionDB
	dialect sqltemplate.Dialect
	authz   *authorizer
}

type secureValueRow struct {
	UID         string
	Namespace   string
	Name        string
	Title       string
	Salt        string
	Value       string
	Keeper      string
	Addr        string
	Created     int64
	CreatedBy   string
	Updated     int64
	UpdatedBy   string
	Annotations string // map[string]string
	Labels      string // map[string]string
	APIs        string // []string
}

type secureValueEvent struct {
	Timestamp int64
	Namespace string
	Name      string
	Action    string
	Identity  string // who
	Details   string // the fields that changed (for update)
}

func (v *secureValueRow) toEvent(auth claims.AuthInfo, action string) *secureValueEvent {
	return &secureValueEvent{
		Timestamp: time.Now().UnixMilli(),
		Namespace: v.Namespace,
		Name:      v.Name,
		Action:    action,
		Identity:  auth.GetUID(),
	}
}

// Convert everything (except the value!) to a flat row structure
func toSecureValueRow(sv *secret.SecureValue) (*secureValueRow, error) {
	meta, err := utils.MetaAccessor(sv)
	if err != nil {
		return nil, err
	}
	row := &secureValueRow{
		UID:       string(sv.UID),
		Namespace: sv.Namespace,
		Name:      sv.Name,
		Title:     sv.Spec.Title,
		Value:     sv.Spec.Value,
		Created:   meta.GetCreationTimestamp().UnixMilli(),
		CreatedBy: meta.GetCreatedBy(),
		UpdatedBy: meta.GetUpdatedBy(),
	}
	rv, err := meta.GetResourceVersionInt64()
	if err == nil {
		row.Updated = rv
	}

	if len(sv.Labels) > 0 {
		v, err := json.Marshal(sv.Labels)
		if err != nil {
			return row, err
		}
		row.Labels = string(v)
	}
	if len(sv.Spec.APIs) > 0 {
		v, err := json.Marshal(sv.Spec.APIs)
		if err != nil {
			return row, err
		}
		row.APIs = string(v)
	}
	if len(sv.Annotations) > 0 {
		anno := CleanAnnotations(sv.Annotations)
		if len(anno) > 0 {
			v, err := json.Marshal(anno)
			if err != nil {
				return row, err
			}
			row.Annotations = string(v)
		}
	}

	// Make sure the raw secret is not in the row (yet)
	if sv.Spec.Value != "" {
		if strings.Contains(row.Annotations, sv.Spec.Value) {
			return nil, fmt.Errorf("raw secret found in annotations")
		}
		if strings.Contains(row.Labels, sv.Spec.Value) {
			return nil, fmt.Errorf("raw secret found in labels")
		}
		if strings.Contains(row.APIs, sv.Spec.Value) {
			return nil, fmt.Errorf("raw secret found in apis")
		}
	}
	return row, nil
}

// Create implements SecureValueStore.
func (v *secureValueRow) toK8s() (*secret.SecureValue, error) {
	val := &secret.SecureValue{
		ObjectMeta: metav1.ObjectMeta{
			Name:              v.Name,
			Namespace:         v.Namespace,
			UID:               types.UID(v.UID),
			CreationTimestamp: metav1.NewTime(time.UnixMilli(v.Created).UTC()),
			Labels:            make(map[string]string),
		},
		Spec: secret.SecureValueSpec{
			Title: v.Title,
		},
	}

	if v.APIs != "" {
		err := json.Unmarshal([]byte(v.APIs), &val.Spec.APIs)
		if err != nil {
			return nil, err
		}
	}
	if v.Annotations != "" {
		err := json.Unmarshal([]byte(v.Annotations), &val.Annotations)
		if err != nil {
			return nil, err
		}
	}
	if v.Labels != "" {
		err := json.Unmarshal([]byte(v.Labels), &val.Labels)
		if err != nil {
			return nil, err
		}
	}

	meta, err := utils.MetaAccessor(val)
	if err != nil {
		return nil, err
	}
	meta.SetCreatedBy(v.CreatedBy)
	meta.SetUpdatedBy(v.UpdatedBy)
	meta.SetUpdatedTimestampMillis(v.Updated)
	meta.SetResourceVersionInt64(v.Updated) // yes millis RV
	return val, nil
}

// Create implements SecureValueStore.
func (s *secureStore) Create(ctx context.Context, v *secret.SecureValue) (*secret.SecureValue, error) {
	authInfo, ok := claims.From(ctx)
	if !ok {
		return nil, fmt.Errorf("missing auth info in context")
	}
	if v.Namespace == "" {
		return nil, fmt.Errorf("missing name")
	}
	if v.Name == "" {
		return nil, fmt.Errorf("missing name")
	}
	if v.Spec.Value == "" {
		return nil, fmt.Errorf("missing value")
	}
	err := s.authz.OnCreate(ctx, authInfo, v.Namespace, v.Name)
	if err != nil {
		return nil, err
	}

	// K8s string value only holds seconds
	v.CreationTimestamp = metav1.NewTime(time.Now().UTC().Truncate(time.Second))
	row, err := toSecureValueRow(v)
	if err != nil {
		return nil, err
	}
	row.UID = uuid.NewString()
	row.CreatedBy = authInfo.GetUID()
	row.UpdatedBy = authInfo.GetUID()
	row.Updated = time.Now().UnixMilli() // full precision
	row.Salt, err = util.GetRandomString(10)
	if err != nil {
		return nil, err
	}
	row.Value, err = s.keeper.Encrypt(ctx, SaltyValue{
		Value:  v.Spec.Value,
		Salt:   row.Salt,
		Keeper: row.Keeper,
		Addr:   row.Addr,
	})
	if err != nil {
		return nil, err
	}

	// insert
	req := &createSecureValue{
		SQLTemplate: sqltemplate.New(s.dialect),
		Row:         row,
	}
	q, err := sqltemplate.Execute(sqlSecureValueInsert, req)
	if err != nil {
		return nil, fmt.Errorf("insert template %q: %w", q, err)
	}

	// Add the value and
	err = s.db.WithTransaction(ctx, func(tx *session.SessionTx) error {
		res, err := tx.Exec(ctx, q, req.GetArgs()...)
		if err != nil {
			return err
		}
		count, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if count < 1 {
			return fmt.Errorf("nothing inserted")
		}
		return s.writeEvent(ctx, tx, row.toEvent(authInfo, "CREATE"))
	})
	if err != nil {
		return nil, err
	}
	return row.toK8s()
}

func (s *secureStore) writeEvent(ctx context.Context, tx *session.SessionTx, e *secureValueEvent) error {
	req := &writeEvent{
		SQLTemplate: sqltemplate.New(s.dialect),
		Event:       e,
	}
	q, err := sqltemplate.Execute(sqlSecureValueEvent, req)
	if err != nil {
		return err
	}

	res, err := tx.Exec(ctx, q, req.GetArgs()...)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count < 1 {
		return fmt.Errorf("no event written")
	}
	return nil
}

// Get implements SecureValueStore.
func (s *secureStore) Read(ctx context.Context, ns string, name string) (*secret.SecureValue, error) {
	authInfo, ok := claims.From(ctx)
	if !ok {
		return nil, fmt.Errorf("missing auth info in context")
	}
	v, err := s.get(ctx, ns, name)
	if err != nil {
		return nil, err
	}
	if !s.authz.CanView(ctx, authInfo, v) {
		return nil, fmt.Errorf("permissions")
	}
	return v.toK8s()
}

// Update implements SecureValueStore.
func (s *secureStore) Update(ctx context.Context, obj *secret.SecureValue) (*secret.SecureValue, error) {
	authInfo, ok := claims.From(ctx)
	if !ok {
		return nil, fmt.Errorf("missing auth info in context")
	}
	existing, err := s.get(ctx, obj.Namespace, obj.Name)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("not found")
	}

	changed := []string{}
	value := obj.Spec.Value
	if value != "" {
		oldvalue, err := s.keeper.Decrypt(ctx, SaltyValue{
			Value:  existing.Value,
			Salt:   existing.Salt,
			Keeper: existing.Keeper,
			Addr:   existing.Addr,
		})
		if oldvalue == value && err == nil {
			obj.Spec.Value = "" // no not return it
			value = ""
		} else {
			changed = []string{"value"}
		}
	}

	row, err := toSecureValueRow(obj)
	if err != nil {
		return nil, err
	}
	row.Salt = existing.Salt
	row.Value = existing.Value
	row.Keeper = existing.Keeper
	row.Addr = existing.Addr

	err = s.authz.OnUpdate(ctx, authInfo, row, existing)
	if err != nil {
		return nil, err
	}

	// From immutable annotations
	row.Created = existing.Created
	row.CreatedBy = existing.CreatedBy
	row.UpdatedBy = existing.UpdatedBy

	if existing.Annotations != row.Annotations {
		changed = append(changed, "annotations")
	}
	if existing.APIs != row.APIs {
		changed = append(changed, "apis")
	}
	if existing.Labels != row.Labels {
		changed = append(changed, "labels")
	}
	if existing.Title != row.Title {
		changed = append(changed, "title")
	}
	if existing.Keeper != row.Keeper {
		changed = append(changed, "keeper")
	}
	if existing.Addr != row.Addr {
		changed = append(changed, "addr")
	}

	if len(changed) < 1 {
		return row.toK8s() // The unchanged value
	}

	if value != "" {
		row.Salt, err = util.GetRandomString(10)
		if err != nil {
			return nil, err
		}
		row.Value, err = s.keeper.Encrypt(ctx, SaltyValue{
			Value:  value,
			Salt:   row.Salt,
			Keeper: row.Keeper,
			Addr:   row.Addr,
		})
		if err != nil {
			return nil, err
		}
	}
	row.Updated = time.Now().UnixMilli()
	row.UpdatedBy = authInfo.GetUID()

	// update
	req := &updateSecureValue{
		SQLTemplate: sqltemplate.New(s.dialect),
		Row:         row,
	}
	q, err := sqltemplate.Execute(sqlSecureValueUpdate, req)
	if err != nil {
		return nil, fmt.Errorf("insert template %q: %w", q, err)
	}

	// Update the value and the event log
	err = s.db.WithTransaction(ctx, func(tx *session.SessionTx) error {
		res, err := tx.Exec(ctx, q, req.GetArgs()...)
		if err != nil {
			return err
		}
		count, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if count < 1 {
			return fmt.Errorf("nothing changed")
		}
		evt := row.toEvent(authInfo, "UPDATE")
		evt.Details = strings.Join(changed, ", ")
		return s.writeEvent(ctx, tx, evt)
	})
	if err != nil {
		return nil, err
	}
	return row.toK8s()
}

// Delete implements SecureValueStore.
func (s *secureStore) Delete(ctx context.Context, ns string, name string) (*secret.SecureValue, bool, error) {
	authInfo, ok := claims.From(ctx)
	if !ok {
		return nil, false, fmt.Errorf("missing auth info in context")
	}

	existing, err := s.get(ctx, ns, name)
	if err != nil {
		return nil, false, err
	}
	if existing == nil {
		return nil, false, fmt.Errorf("not found")
	}

	err = s.authz.OnDelete(ctx, authInfo, existing)
	if err != nil {
		return nil, false, err
	}

	deleted := false
	err = s.db.WithTransaction(ctx, func(tx *session.SessionTx) error {
		res, err := tx.Exec(ctx, "DELETE FROM secure_value WHERE uid=?", existing.UID)
		if err != nil {
			return err
		}
		count, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if count == 1 {
			deleted = true
			evt := existing.toEvent(authInfo, "DELETED")
			return s.writeEvent(ctx, tx, evt)
		}
		return nil
	})
	return nil, deleted, err
}

// List implements SecureValueStore.
func (s *secureStore) List(ctx context.Context, ns string, options *internalversion.ListOptions) (*secret.SecureValueList, error) {
	authInfo, ok := claims.From(ctx)
	if !ok {
		return nil, fmt.Errorf("missing auth info in context")
	}
	req := &listSecureValues{
		SQLTemplate: sqltemplate.New(s.dialect),
		Request: secureValueRow{
			Namespace: ns,
		},
	}
	q, err := sqltemplate.Execute(sqlSecureValueList, req)
	if err != nil {
		return nil, fmt.Errorf("list template %q: %w", q, err)
	}

	selector := options.LabelSelector
	if selector == nil {
		selector = labels.Everything()
	}

	row := &secureValueRow{}
	list := &secret.SecureValueList{}
	rows, err := s.db.Query(ctx, q, req.GetArgs()...)
	if err != nil {
		return nil, fmt.Errorf("list template %q: %w", q, err)
	}
	defer func() {
		_ = rows.Close()
	}()
	for rows.Next() {
		err = rows.Scan(&row.UID,
			&row.Namespace, &row.Name, &row.Title,
			&row.Salt, &row.Value,
			&row.Keeper, &row.Addr,
			&row.Created, &row.CreatedBy,
			&row.Updated, &row.UpdatedBy,
			&row.Annotations, &row.Labels,
			&row.APIs,
		)
		if err != nil {
			return nil, err
		}

		// Skip the values we can not read
		if !s.authz.CanView(ctx, authInfo, row) {
			continue
		}

		obj, err := row.toK8s()
		if err != nil {
			return nil, err
		}
		if selector.Matches(labels.Set(obj.Labels)) {
			list.Items = append(list.Items, *obj)
		}
	}
	return list, nil // nothing
}

// Decrypt implements SecureValueStore.
func (s *secureStore) Decrypt(ctx context.Context, ns string, name string) (*secret.SecureValue, error) {
	authInfo, ok := claims.From(ctx)
	if !ok {
		return nil, fmt.Errorf("missing auth info in context")
	}

	row, err := s.get(ctx, ns, name)
	if err != nil {
		return nil, err
	}

	// Skip the values we can not read
	if !s.authz.CanDecrypt(ctx, authInfo, row) {
		return nil, fmt.Errorf("no permissions")
	}

	v, err := row.toK8s()
	if err != nil {
		return nil, err
	}
	v.Spec.Value, err = s.keeper.Decrypt(ctx, SaltyValue{
		Value:  row.Value,
		Salt:   row.Salt,
		Keeper: row.Keeper,
		Addr:   row.Addr,
	})
	return v, err
}

// History implements SecureValueStore.
func (s *secureStore) History(ctx context.Context, ns string, name string, continueToken string) (*secret.SecureValueActivityList, error) {
	// Verify same permissions as Read
	_, err := s.get(ctx, ns, name)
	if err != nil {
		return nil, err
	}

	req := readHistory{
		SQLTemplate: sqltemplate.New(s.dialect),
		Namespace:   ns,
		Name:        name,
	}

	q, err := sqltemplate.Execute(sqlSecureValueHistory, req)
	if err != nil {
		return nil, fmt.Errorf("list template %q: %w", q, err)
	}

	rows, err := s.db.Query(ctx, q, req.GetArgs()...)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	list := &secret.SecureValueActivityList{}
	for rows.Next() {
		event := secret.SecureValueActivity{}
		err = rows.Scan(
			&event.Timestamp, &event.Action,
			&event.Identity, &event.Details,
		)
		if err != nil {
			break
		}
		list.Items = append(list.Items, event)
	}
	return list, err
}

func (s *secureStore) get(ctx context.Context, ns string, name string) (*secureValueRow, error) {
	req := &listSecureValues{
		SQLTemplate: sqltemplate.New(s.dialect),
		Request: secureValueRow{
			Namespace: ns,
			Name:      name,
		},
	}
	q, err := sqltemplate.Execute(sqlSecureValueList, req)
	if err != nil {
		return nil, fmt.Errorf("list template %q: %w", q, err)
	}

	rows, err := s.db.Query(ctx, q, req.GetArgs()...)
	if err != nil {
		return nil, fmt.Errorf("list template %q: %w", q, err)
	}
	defer func() {
		_ = rows.Close()
	}()
	if rows.Next() {
		row := &secureValueRow{}
		err = rows.Scan(&row.UID,
			&row.Namespace, &row.Name, &row.Title,
			&row.Salt, &row.Value,
			&row.Keeper, &row.Addr,
			&row.Created, &row.CreatedBy,
			&row.Updated, &row.UpdatedBy,
			&row.Annotations, &row.Labels,
			&row.APIs,
		)
		return row, err
	}
	return nil, fmt.Errorf("not found")
}
