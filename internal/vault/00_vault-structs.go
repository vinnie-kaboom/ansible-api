package vault

import (
	"sync"

	vault "github.com/hashicorp/vault/api"
)

type Client struct {
	client      *vault.Client
	roleID      string
	secretID    string
	tokenMutex  sync.RWMutex
	stopChan    chan struct{}
	VaultClient *vault.Client
}
