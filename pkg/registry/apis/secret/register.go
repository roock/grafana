package secret

import (
	secret "github.com/grafana/grafana/pkg/apis/secret/v0alpha1"
	grafanarest "github.com/grafana/grafana/pkg/apiserver/rest"
	"github.com/grafana/grafana/pkg/services/apiserver/builder"
	"github.com/grafana/grafana/pkg/services/featuremgmt"
	secretstore "github.com/grafana/grafana/pkg/storage/secret"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/apiserver/pkg/registry/generic"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	common "k8s.io/kube-openapi/pkg/common"
)

var _ builder.APIGroupBuilder = (*SecretAPIBuilder)(nil)

// This is used just so wire has something unique to return
type SecretAPIBuilder struct {
	store secretstore.SecureValueStore
}

func NewSecretAPIBuilder(store secretstore.SecureValueStore) *SecretAPIBuilder {
	return &SecretAPIBuilder{store}
}

func RegisterAPIService(features featuremgmt.FeatureToggles,
	apiregistration builder.APIRegistrar,
	store secretstore.SecureValueStore,
) *SecretAPIBuilder {
	if !features.IsEnabledGlobally(featuremgmt.FlagGrafanaAPIServerWithExperimentalAPIs) {
		return nil // skip registration unless opting into experimental apis
	}

	builder := NewSecretAPIBuilder(store)
	apiregistration.RegisterAPI(builder)
	return builder
}

func (b *SecretAPIBuilder) GetGroupVersion() schema.GroupVersion {
	return secret.SecureValuesResourceInfo.GroupVersion()
}

func (b *SecretAPIBuilder) InstallSchema(scheme *runtime.Scheme) error {
	secret.AddKnownTypes(scheme, secret.VERSION)

	// Link this version to the internal representation.
	// This is used for server-side-apply (PATCH), and avoids the error:
	// "no kind is registered for the type"
	secret.AddKnownTypes(scheme, runtime.APIVersionInternal)

	metav1.AddToGroupVersion(scheme, secret.SchemeGroupVersion)
	return scheme.SetVersionPriority(secret.SchemeGroupVersion)
}

func (b *SecretAPIBuilder) GetAPIGroupInfo(
	scheme *runtime.Scheme,
	codecs serializer.CodecFactory, // pointer?
	_ generic.RESTOptionsGetter,
	_ grafanarest.DualWriteBuilder,
) (*genericapiserver.APIGroupInfo, error) {
	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(secret.GROUP, scheme, metav1.ParameterCodec, codecs)

	resource := secret.SecureValuesResourceInfo
	storage := map[string]rest.Storage{}
	storage[resource.StoragePath()] = &secretStorage{
		store:          b.store,
		resource:       resource,
		tableConverter: resource.TableConverter(),
	}
	storage[resource.StoragePath("view")] = &secretView{
		store: b.store,
	}

	apiGroupInfo.VersionedResourcesStorageMap[secret.VERSION] = storage
	return &apiGroupInfo, nil
}

func (b *SecretAPIBuilder) GetOpenAPIDefinitions() common.GetOpenAPIDefinitions {
	return secret.GetOpenAPIDefinitions
}

func (b *SecretAPIBuilder) GetAuthorizer() authorizer.Authorizer {
	// TODO... who can create secrets? must be multi-tenant first
	return nil // start with the default authorizer
}

// Register additional routes with the server
func (b *SecretAPIBuilder) GetAPIRoutes() *builder.APIRoutes {
	return nil
}
