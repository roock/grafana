package secret

import (
	"testing"
	"text/template"

	"github.com/grafana/grafana/pkg/storage/unified/sql/sqltemplate/mocks"
)

func TestSecureValuesQueries(t *testing.T) {
	mocks.CheckQuerySnapshots(t, mocks.TemplateTestSetup{
		RootDir: "testdata",
		Templates: map[*template.Template][]mocks.TemplateTestCase{
			sqlSecureValueInsert: {
				{
					Name: "simple",
					Data: &createSecureValue{
						SQLTemplate: mocks.NewTestingSQLTemplate(),
						Row: &secureValueRow{
							UID:         "abc",
							Namespace:   "ns",
							Name:        "name",
							Title:       "title",
							Salt:        "salt",
							Value:       "value",
							Keeper:      "keeper",
							Addr:        "addr",
							Created:     1234,
							CreatedBy:   "user:ryan",
							Updated:     5678,
							UpdatedBy:   "user:cameron",
							Annotations: `{"x":"XXXX"}`,
							Labels:      `{"a":"AAA", "b", "BBBB"}`,
							APIs:        `["aaa", "bbb", "ccc"]`,
						},
					},
				},
			},
			sqlSecureValueUpdate: {
				{
					Name: "simple",
					Data: &updateSecureValue{
						SQLTemplate: mocks.NewTestingSQLTemplate(),
						Row: &secureValueRow{
							UID:         "uid",
							Namespace:   "ns",
							Name:        "name",
							Title:       "ttt",
							Salt:        "salt",
							Value:       "vvv",
							Keeper:      "keeper",
							Addr:        "addr",
							Created:     1234,
							CreatedBy:   "user:ryan",
							Updated:     5678,
							UpdatedBy:   "user:cameron",
							Annotations: `{"x":"XXXX"}`,
							Labels:      `{"a":"AAA", "b", "BBBB"}`,
							APIs:        `["aaa", "bbb", "ccc"]`,
						},
					},
				},
			},
		}})
}
