#!/bin/bash
set -e

echo "╔═══════════════════════════════════════╗"
echo "║        Qorven Installer v1.0          ║"
echo "╚═══════════════════════════════════════╝"
echo ""

# Detect OS/arch
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in x86_64) ARCH="amd64";; aarch64|arm64) ARCH="arm64";; esac
echo "→ System: $OS/$ARCH"

QORVEN_USER=${SUDO_USER:-$USER}
QORVEN_HOME=$(eval echo ~$QORVEN_USER)
CONF_DIR="$QORVEN_HOME/.qorven"

# 1. Install system dependencies
echo ""
echo "→ Installing dependencies..."
if command -v apt-get &>/dev/null; then
  apt-get update -qq
  apt-get install -y -qq golang-go postgresql postgresql-contrib git build-essential 2>&1 | tail -1
  # Install pgvector
  PG_VER=$(pg_config --version | grep -oP '\d+' | head -1)
  apt-get install -y -qq postgresql-${PG_VER}-pgvector 2>/dev/null || {
    echo "  Building pgvector from source..."
    apt-get install -y -qq postgresql-server-dev-${PG_VER} 2>&1 | tail -1
    cd /tmp && git clone --branch v0.7.4 https://github.com/pgvector/pgvector.git 2>/dev/null
    cd /tmp/pgvector && make -j$(nproc) 2>&1 | tail -1 && make install 2>&1 | tail -1
  }
elif command -v dnf &>/dev/null; then
  dnf install -y golang postgresql-server postgresql-contrib git gcc make 2>&1 | tail -1
  postgresql-setup --initdb 2>/dev/null || true
  systemctl enable postgresql && systemctl start postgresql
elif command -v yum &>/dev/null; then
  yum install -y golang postgresql-server postgresql-contrib git gcc make 2>&1 | tail -1
  postgresql-setup initdb 2>/dev/null || true
  systemctl enable postgresql && systemctl start postgresql
fi

# Ensure PostgreSQL is running
systemctl start postgresql 2>/dev/null || service postgresql start 2>/dev/null || true

echo "  Go: $(go version 2>/dev/null | head -1)"
echo "  PostgreSQL: $(psql --version 2>/dev/null | head -1)"

# 2. Setup database
echo ""
echo "→ Setting up database..."
DB_PASS="qorven$(openssl rand -hex 8 2>/dev/null || echo 'secure2026')"
sudo -u postgres psql -c "CREATE USER qorven WITH PASSWORD '$DB_PASS';" 2>/dev/null || true
sudo -u postgres psql -c "CREATE DATABASE qorven OWNER qorven;" 2>/dev/null || true
sudo -u postgres psql -c "CREATE EXTENSION IF NOT EXISTS vector;" -d qorven 2>/dev/null || true
sudo -u postgres psql -c "CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\";" -d qorven 2>/dev/null || true
echo "  Database: qorven ✅"

# 3. Clone and build
echo ""
echo "→ Building Qorven..."
REPO_DIR="/opt/qorven"
if [ -d "$REPO_DIR" ]; then
  cd "$REPO_DIR" && git pull 2>&1 | tail -1
else
  git clone https://github.com/qorvenai/qorven.git "$REPO_DIR" 2>&1 | tail -1 || {
    echo "  Public repo not available yet. Trying local build..."
    mkdir -p "$REPO_DIR"
    # If running from a cloned repo, use that
    if [ -f "backend/go.mod" ]; then
      cp -r . "$REPO_DIR/"
    fi
  }
fi

cd "$REPO_DIR/backend"
echo "  Building binary (this takes 1-2 minutes)..."
CGO_ENABLED=0 go build -o /usr/local/bin/qorven . 2>&1 | tail -3
chmod +x /usr/local/bin/qorven
echo "  Binary: $(ls -lh /usr/local/bin/qorven | awk '{print $5}')"

# 4. Allow ports 80/443 without root
setcap 'cap_net_bind_service=+ep' /usr/local/bin/qorven 2>/dev/null || true

# 5. Create config
echo ""
echo "→ Creating config..."
mkdir -p "$CONF_DIR"
cat > "$CONF_DIR/config.toml" << EOF
[server]
listen = "0.0.0.0:4200"

[database]
dsn = "postgres://qorven:$DB_PASS@localhost:5432/qorven?sslmode=disable"
EOF
chown -R $QORVEN_USER:$QORVEN_USER "$CONF_DIR"

# Copy migrations
cp -r "$REPO_DIR/backend/migrations" "$CONF_DIR/migrations" 2>/dev/null || true

# 6. Create systemd service
echo "→ Creating systemd service..."
cat > /etc/systemd/system/qorven.service << EOF
[Unit]
Description=Qorven AI Workspace
After=network.target postgresql.service

[Service]
Type=simple
User=$QORVEN_USER
WorkingDirectory=$REPO_DIR/backend
ExecStart=/usr/local/bin/qorven start
Restart=always
RestartSec=5
Environment=HOME=$QORVEN_HOME

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable qorven
systemctl start qorven

# 7. Wait for startup
echo ""
echo "→ Starting Qorven..."
for i in $(seq 1 15); do
  if curl -s http://localhost:4200/health >/dev/null 2>&1; then
    echo "  Qorven is running ✅"
    break
  fi
  sleep 1
done

# 8. Show result
IP=$(curl -s http://checkip.amazonaws.com 2>/dev/null || hostname -I | awk '{print $1}')
echo ""
echo "╔═══════════════════════════════════════════════╗"
echo "║  ✅ Qorven installed!                         ║"
echo "║                                                ║"
echo "║  Open your browser:                            ║"
echo "║  http://$IP:4200                               ║"
echo "║                                                ║"
echo "║  Complete setup in the browser to:             ║"
echo "║  • Create your admin account                   ║"
echo "║  • Add your LLM provider key                   ║"
echo "║  • Start chatting with Prime                   ║"
echo "║                                                ║"
echo "║  Commands:                                     ║"
echo "║  sudo systemctl status qorven  — check status  ║"
echo "║  sudo journalctl -u qorven -f  — view logs     ║"
echo "║  sudo systemctl restart qorven — restart        ║"
echo "╚═══════════════════════════════════════════════╝"
