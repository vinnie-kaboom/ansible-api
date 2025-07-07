path "secret/data/ansible/*" {
  capabilities = ["read", "list"]
}

path "secret/metadata/ansible/*" {
  capabilities = ["list"]
}

path "kv/data/ansible/*" {
  capabilities = ["read", "list"]
}

path "kv/metadata/ansible/*" {
  capabilities = ["list"]
} 