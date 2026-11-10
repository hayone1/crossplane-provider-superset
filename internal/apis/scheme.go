// Package apis lets the generated reference resolvers create managed resources by API
// group, version and kind. The resolvers use it instead of importing the API packages of
// other groups, because those imports would form cycles: an Application refers to Roles,
// and a Role refers to an Application.
package apis

import (
	xpresource "github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var s = runtime.NewScheme()

// BuildScheme registers the provider's API types. The provider calls it once at startup,
// before it resolves any reference.
func BuildScheme(sb runtime.SchemeBuilder) error {
	return errors.Wrap(sb.AddToScheme(s), "failed to register the GVKs with the runtime scheme")
}

// GetManagedResource returns a new managed resource and a new list of the given API group,
// version and kinds.
func GetManagedResource(group, version, kind, listKind string) (xpresource.Managed, xpresource.ManagedList, error) {
	gv := schema.GroupVersion{Group: group, Version: version}

	obj, err := s.New(gv.WithKind(kind))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to get a new API object of GVK %q from the runtime scheme", gv.WithKind(kind))
	}
	m, ok := obj.(xpresource.Managed)
	if !ok {
		return nil, nil, errors.Errorf("%q is not a managed resource", gv.WithKind(kind))
	}

	listObj, err := s.New(gv.WithKind(listKind))
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to get a new API object of GVK %q from the runtime scheme", gv.WithKind(listKind))
	}
	l, ok := listObj.(xpresource.ManagedList)
	if !ok {
		return nil, nil, errors.Errorf("%q is not a managed resource list", gv.WithKind(listKind))
	}
	return m, l, nil
}
