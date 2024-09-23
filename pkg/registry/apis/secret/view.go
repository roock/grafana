package secret

import (
	"context"
	"net/http"

	secret "github.com/grafana/grafana/pkg/apis/secret/v0alpha1"
	secretstore "github.com/grafana/grafana/pkg/storage/secret"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
)

// The DTO returns everything the UI needs in a single request
type secretView struct {
	store secretstore.SecureValueStore
}

var (
	_ rest.Connecter       = (*secretView)(nil)
	_ rest.StorageMetadata = (*secretView)(nil)
)

func (r *secretView) New() runtime.Object {
	return &secret.SecureValue{}
}

func (r *secretView) Destroy() {
}

func (r *secretView) ConnectMethods() []string {
	return []string{"GET"}
}

func (r *secretView) NewConnectOptions() (runtime.Object, bool, string) {
	return nil, false, ""
}

func (r *secretView) ProducesMIMETypes(verb string) []string {
	return []string{"text/plain"}
}

func (r *secretView) ProducesObject(verb string) interface{} {
	return &secret.SecureValue{}
}

func (r *secretView) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	ns := request.NamespaceValue(ctx)
	val, err := r.store.Decrypt(ctx, ns, name)
	if err != nil {
		return nil, err
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if true {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(val.Spec.Value)) // the raw value...
			return
		}

		responder.Object(http.StatusOK, val)
	}), nil
}
