#!/usr/bin/env bash
echo "SSH_CLIENT=$SSH_CLIENT"
echo "--- who ---"
who 2>/dev/null | head -5
echo "--- sshd effective ---"
sshd -T 2>/dev/null | grep -Ei '^(passwordauthentication|permitrootlogin|pubkeyauthentication|port) '
echo "--- config files ---"
grep -rEi '^[[:space:]]*(PasswordAuthentication|PermitRootLogin)' /etc/ssh/sshd_config /etc/ssh/sshd_config.d/ 2>/dev/null
echo "--- key ---"
ls -l /root/.ssh/authorized_keys 2>/dev/null
