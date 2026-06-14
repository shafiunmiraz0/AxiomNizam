package repositories

// Compile-time interface satisfaction checks.
// These ensure concrete types implement the repository interfaces.

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/iam/pgstore"
)

var _ RealmRepository = (*pgstore.Store)(nil)
var _ ClientRepository = (*pgstore.Store)(nil)
var _ UserRepository = (*pgstore.Store)(nil)
var _ RoleRepository = (*pgstore.Store)(nil)
var _ GroupRepository = (*pgstore.Store)(nil)
var _ ClientScopeRepository = (*pgstore.Store)(nil)
var _ IdentityProviderRepository = (*pgstore.Store)(nil)
var _ SessionRepository = (*pgstore.Store)(nil)
var _ EventRepository = (*pgstore.Store)(nil)
