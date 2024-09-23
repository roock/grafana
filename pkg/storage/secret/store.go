package secret

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/grafana/authlib/claims"
	"github.com/grafana/grafana/pkg/apimachinery/utils"
	secret "github.com/grafana/grafana/pkg/apis/secret/v0alpha1"
	"github.com/grafana/grafana/pkg/infra/db"
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
	History(ctx context.Context, ns string, name string, continueToken string) (*secret.SecureValueActivity, error)
}

func ProvideSecureValueStore(db db.DB, keeper SecretKeeper, cfg *setting.Cfg) (SecureValueStore, error) {
	// Run SQL migrations
	err := MigrateSecretStore(context.Background(), db.GetEngine(), cfg)
	if err != nil {
		return nil, err
	}

	// One version of DB?
	return &secureStore{
		keeper:  keeper,
		db:      db,
		dialect: sqltemplate.DialectForDriver(string(db.GetDBType())),
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

	//go:embed *.sql
	sqlTemplatesFS embed.FS

	sqlTemplates = template.Must(template.New("sql").ParseFS(sqlTemplatesFS, `*.sql`))

	// The SQL Commands
	sqlSecureValueInsert = mustTemplate("secure_value_insert.sql")
	sqlSecureValueUpdate = mustTemplate("secure_value_update.sql")
	sqlSecureValueList   = mustTemplate("secure_value_list.sql")

	// Exclude these annotations
	skipAnnotations = map[string]bool{
		"kubectl.kubernetes.io/last-applied-configuration": true, // force server side apply
		utils.AnnoKeyCreatedBy:                             true,
		utils.AnnoKeyUpdatedBy:                             true,
		utils.AnnoKeyUpdatedTimestamp:                      true,
	}
)

func mustTemplate(filename string) *template.Template {
	if t := sqlTemplates.Lookup(filename); t != nil {
		return t
	}
	panic(fmt.Sprintf("template file not found: %s", filename))
}

type secureStore struct {
	keeper  SecretKeeper
	db      db.DB
	dialect sqltemplate.Dialect
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

type createSecureValue struct {
	sqltemplate.SQLTemplate
	Row *secureValueRow
}

func (r createSecureValue) Validate() error {
	return nil // TODO
}

type updateSecureValue struct {
	sqltemplate.SQLTemplate
	Row *secureValueRow
}

func (r updateSecureValue) Validate() error {
	return nil // TODO
}

// Create implements SecureValueStore.
func (s *secureStore) Create(ctx context.Context, v *secret.SecureValue) (*secret.SecureValue, error) {
	authInfo, ok := claims.From(ctx)
	if !ok {
		return nil, fmt.Errorf("missing auth info in context")
	}
	if v.Name == "" {
		return nil, fmt.Errorf("missing name")
	}
	if v.Spec.Value == "" {
		return nil, fmt.Errorf("missing value")
	}

	v.CreationTimestamp = metav1.NewTime(time.Now().UTC().Truncate(time.Second)) // seconds
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

	res, err := s.db.GetSqlxSession().Exec(ctx, q, req.GetArgs()...)
	if err != nil {
		return nil, err
	}

	count, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count == 1 {
		return row.toK8s()
	}
	return nil, fmt.Errorf("error creating row")
}

// Get implements SecureValueStore.
func (s *secureStore) Read(ctx context.Context, ns string, name string) (*secret.SecureValue, error) {
	v, err := s.get(ctx, ns, name)
	if err != nil {
		return nil, err
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

	// From immutable annotations
	row.Created = existing.Created
	row.CreatedBy = existing.CreatedBy
	row.UpdatedBy = existing.UpdatedBy

	if value == "" && cmp.Equal(row, existing) {
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

	res, err := s.db.GetSqlxSession().Exec(ctx, q, req.GetArgs()...)
	if err != nil {
		return nil, err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count == 1 {
		return row.toK8s()
	}
	return nil, fmt.Errorf("error updating row")
}

// Delete implements SecureValueStore.
func (s *secureStore) Delete(ctx context.Context, ns string, name string) (*secret.SecureValue, bool, error) {
	existing, err := s.get(ctx, ns, name)
	if err != nil {
		return nil, false, err
	}
	if existing == nil {
		return nil, false, fmt.Errorf("not found")
	}

	res, err := s.db.GetSqlxSession().Exec(ctx, "DELETE FROM secure_value WHERE uid=?", existing.UID)
	if err != nil {
		return nil, false, err
	}
	count, err := res.RowsAffected()
	if count > 0 {
		return nil, true, nil
	}
	return nil, false, err // ????
}

type listSecureValues struct {
	sqltemplate.SQLTemplate
	Request secureValueRow
}

func (r listSecureValues) Validate() error {
	return nil // TODO
}

// List implements SecureValueStore.
func (s *secureStore) List(ctx context.Context, ns string, options *internalversion.ListOptions) (*secret.SecureValueList, error) {
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
	rows, err := s.db.GetSqlxSession().Query(ctx, q, req.GetArgs()...)
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
	row, err := s.get(ctx, ns, name)
	if err != nil {
		return nil, err
	}

	// TODO!!!
	if row.APIs != "" {
		fmt.Printf("MAKE SURE ctx is an app that can read: %s\n", row.APIs)
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
func (s *secureStore) History(ctx context.Context, ns string, name string, continueToken string) (*secret.SecureValueActivity, error) {
	panic("unimplemented")
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

	rows, err := s.db.GetSqlxSession().Query(ctx, q, req.GetArgs()...)
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
