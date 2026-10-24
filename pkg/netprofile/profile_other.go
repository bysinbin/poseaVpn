//go:build !windows

package netprofile

// NetworkCategory defines the network profile category.
type NetworkCategory string

const (
	CategoryPublic  NetworkCategory = "Public"
	CategoryPrivate NetworkCategory = "Private"
	CategoryDomain  NetworkCategory = "DomainAuthenticated"
)

// SetInterfaceCategory is a no-op on non-Windows platforms.
// macOS does not use Windows Firewall network categories.
func SetInterfaceCategory(interfaceAlias string, category NetworkCategory) error {
	return nil
}

// GetInterfaceCategory returns CategoryPrivate on non-Windows platforms.
func GetInterfaceCategory(interfaceAlias string) (string, error) {
	return string(CategoryPrivate), nil
}
