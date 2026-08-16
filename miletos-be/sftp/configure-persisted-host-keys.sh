#!/bin/sh
set -eu

key_directory=/var/lib/miletos-sftp-host-keys
sshd_config=/etc/ssh/sshd_config

install -d -m 0700 -o root -g root "$key_directory"

ensure_host_key() {
    key_name=$1
    key_type=$2
    key_bits=${3:-}
    private_key="$key_directory/$key_name"
    public_key="$private_key.pub"

    if [ ! -e "$private_key" ]; then
        if [ -n "$key_bits" ]; then
            ssh-keygen -q -t "$key_type" -b "$key_bits" -N '' -f "$private_key"
        else
            ssh-keygen -q -t "$key_type" -N '' -f "$private_key"
        fi
    fi
    if [ ! -s "$private_key" ]; then
        echo "Persisted SSH host key is empty: $private_key" >&2
        exit 1
    fi
    if [ ! -s "$public_key" ]; then
        ssh-keygen -y -f "$private_key" > "$public_key"
    fi

    chown root:root "$private_key" "$public_key"
    chmod 0600 "$private_key"
    chmod 0644 "$public_key"
}

ensure_host_key ssh_host_ecdsa_key ecdsa 256
ensure_host_key ssh_host_ed25519_key ed25519
ensure_host_key ssh_host_rsa_key rsa 4096

temporary_config=$(mktemp)
sed '/^[[:space:]]*HostKey[[:space:]]/d' "$sshd_config" > "$temporary_config"
cat >> "$temporary_config" <<EOF
HostKey $key_directory/ssh_host_ecdsa_key
HostKey $key_directory/ssh_host_ed25519_key
HostKey $key_directory/ssh_host_rsa_key
EOF
cat "$temporary_config" > "$sshd_config"
rm -f "$temporary_config"

sshd -t
