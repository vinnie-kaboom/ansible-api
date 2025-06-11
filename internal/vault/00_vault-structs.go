package vault

import (
	vault "github.com/hashicorp/vault/api"
)

type Client struct {
	client *vault.Client
}
