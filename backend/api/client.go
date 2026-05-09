package api

import (
	"github.com/dell-infra-manager/backend/crypto"
	"github.com/dell-infra-manager/backend/models"
	"github.com/dell-infra-manager/backend/redfish"
	"github.com/jmoiron/sqlx"
)

// buildClient looks up a server by id, decrypts its iDRAC password, and returns
// a configured Redfish client.
func buildClient(db *sqlx.DB, serverID string) (*redfish.Client, error) {
	_, client, err := loadServerAndClient(db, serverID)
	return client, err
}

// loadServerAndClient returns both the server row and a Redfish client. Use
// when handlers need fields like Name in addition to the client (e.g. for bulk
// result reporting).
func loadServerAndClient(db *sqlx.DB, serverID string) (*models.Server, *redfish.Client, error) {
	var s models.Server
	if err := db.Get(&s, `SELECT * FROM servers WHERE id = ?`, serverID); err != nil {
		return nil, nil, err
	}
	password, err := crypto.Decrypt(s.Password)
	if err != nil {
		return nil, nil, err
	}
	return &s, redfish.NewClient(s.Hostname, s.Port, s.Username, password, s.TLSVerify), nil
}
