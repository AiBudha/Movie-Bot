# Autofilter Bot

A powerful and customizable Telegram bot written in Go for automatically filtering and indexing files.

## Commands

### Bot Administration & Files

- `start` - Check if the bot is alive.
- `help` - How to use the bot.
- `about` - About the bot.
- `settings` - Bot configuration (Admin only).
- `index` - Import files from channels (Admin only).
- `broadcast` - Send messages to all users (Admin only).
- `platforms` - List supported streaming platforms.
- `subscribe` - Subscribe to OTT updates in PM.
- `unsubscribe` - Unsubscribe from OTT updates in PM.
- `latest` - Get latest releases now with paginated buttons.
- `sendnow` - Trigger background release check immediately (Admin only).

### Group Moderation & Administration

- `ban` / `unban` - Ban or unban a user in a group chat.
- `mute` / `unmute` - Mute or unmute a user in a group chat.
- `kick` / `kickme` - Kick a user from a group, or kick yourself.
- `promote` / `demote` - Promote a user to administrator or demote them.
- `title` - Set custom title for an administrator.
- `admins` / `adminlist` - List administrators in the group chat.
- `warn` / `unwarn` - Add or remove warning from a user.
- `warns` - Display active warnings for a user.
- `resetwarns` / `clearwarns` - Clear all warnings of a user.
- `setwarnlimit` / `setwarnmode` - Configure warning limit and action type.
- `rules` / `setrules` / `clearrules` - Show, set, or delete group chat rules.
- `welcome` / `setwelcome` / `clearwelcome` - Customize or disable welcome greetings for new members.
- `locks` / `lock` / `unlock` - Show, enable, or disable locks on specific message types.
- `pin` / `unpin` - Pin or unpin messages in the group.
- `captcha` / `captchatime` - Enable/disable captcha verification and configure timeout duration.
- `antiraid` - Toggle anti-raid security protection.
- `setflood` - Configure automatic anti-flood message threshold.

## Features

- **Automatic File Filtering:** Automatically saves and filters files from configured channels.
- **Large Volume Indexing:** Efficiently index large amounts of files.
- **Customizable:** Extensive configuration options via environment variables.
- **Multi-Database Support:** Support for multiple MongoDB databases.
- **Force Subscribe:** Optional requirement for users to subscribe to channels before using the bot.
- **OTT Releases Tracker & Auto-Poster:** Scrapes new releases from TMDB, JustWatch GQL/Web, and OTTRelease.com.
- **Background Release Scheduler:** Runs periodically and posts new releases to configured channels or chats.
- **Daily Digest Generator:** Sends a daily summary of all movie/series releases automatically every 24 hours.

## Deployment

This bot can be deployed to a VPS (Virtual Private Server) or a cloud platform like Render.

### Prerequisites

- **Go**: Version 1.24+ (if running/building locally).
- **MongoDB**: A running instance (local or MongoDB Atlas).
- **Docker**: (Optional) if deploying via containers.

### Environment Variables

Create a `.env` file in the root directory (you can copy `.env.example` as a template) and configure the following variables:

```env
# Bot configuration
BOT_TOKEN=your_telegram_bot_token
ADMINS=123456789,987654321
MONGODB_URI=mongodb+srv://...
DATABASE_NAME=movie_bot
COLLECTION_NAME=files
LOG_LEVEL=info
LOG_CHANNEL=-100xxxxxxxxxx

# Telegram API credentials (for indexing/sessions)
APP_ID=12345
APP_HASH=your_telegram_app_hash

# OTT Updates Configuration
TMDB_API_KEY=your_tmdb_api_key_here
JUSTWATCH_COUNTRY=IN
UPDATE_INTERVAL_HOURS=2
OTT_CHANNEL_ID=-100xxxxxxxxxx
```

---

### VPS Deployment Methods

#### Method A: Docker Compose (Recommended)

1. Ensure `docker` and `docker-compose` (or `docker compose`) are installed on your VPS.
2. Configure your `.env` file in the project folder.
3. Build and start the container in background mode:
   ```bash
   docker compose up --build -d
   ```
4. Check the logs to ensure the bot started successfully:
   ```bash
   docker compose logs -f
   ```

#### Method B: Systemd Service (Process Manager)

If you want to run the compiled Go binary directly on your VPS as a background service:

1. Build the binary on your server:
   ```bash
   go build -o bot main.go
   ```
2. Create a systemd service file:
   ```bash
   sudo nano /etc/systemd/system/moviebot.service
   ```
3. Paste the following configuration (replace `/path/to/bot` with the actual path to your bot folder):
   ```ini
   [Unit]
   Description=Movie Filter Bot Service
   After=network.target

   [Service]
   Type=simple
   User=root
   WorkingDirectory=/path/to/bot
   ExecStart=/path/to/bot/bot
   Restart=always
   RestartSec=5

   [Install]
   WantedBy=multi-user.target
   ```
4. Start and enable the service:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl start moviebot
   sudo systemctl enable moviebot
   ```
5. Check status and logs:
   ```bash
   sudo systemctl status moviebot
   journalctl -u moviebot -f
   ```

#### Method C: PM2

If you prefer PM2 for process management:

1. Build the binary: `go build -o bot main.go`.
2. Start the binary with PM2:
   ```bash
   pm2 start ./bot --name "moviebot"
   ```
3. Save the process list: `pm2 save`.

---

#### Detailed VPS Host Tutorial

Deploying the bot on a fresh VPS (Ubuntu 20.04/22.04) is straightforward. Follow these steps:

1. **Provision a VPS** – Choose a provider (DigitalOcean, Linode, Hetzner, etc.) and create a server with at least 1 CPU and 1 GB RAM.
2. **Access the server** – Open an SSH session:
   ```bash
   ssh root@your-vps-ip
   ```
3. **Update the system** – Ensure packages are up‑to‑date:
   ```bash
   apt update && apt upgrade -y
   ```
4. **Install required tools** – Go, Docker, Git, and a process manager (systemd is built‑in):
   ```bash
   # Install Go (1.24+)
   apt install -y golang-go
   # Install Docker
   apt install -y docker.io docker-compose
   # Enable Docker services
   systemctl enable --now docker
   # Install Git
   apt install -y git
   ```
5. **Create a non‑root user** (optional but recommended):
   ```bash
   adduser botuser
   usermod -aG docker botuser
   ```
   Then switch to this user for future steps:
   ```bash
   su - botuser
   ```
6. **Clone the repository**:
   ```bash
   git clone https://github.com/yourusername/Moviebot001-main.git
   cd Moviebot001-main
   ```
7. **Configure environment variables** – Copy the example file and edit:
   ```bash
   cp .env.example .env
   nano .env   # set BOT_TOKEN, ADMINS, MONGODB_URI, etc.
   ```
8. **Choose a deployment method** – The simplest is Docker Compose (see Method A). If you prefer a native binary, use the Systemd service (Method B) or the following manual steps:
   ```bash
   go build -o moviebot main.go
   ./moviebot &   # runs in background; consider using tmux or screen
   ```
9. **Set up a firewall** (UFW) to allow only required ports:
   ```bash
   apt install -y ufw
   ufw allow OpenSSH
   ufw allow 10000/tcp   # optional health‑check port
   ufw enable
   ```
10. **Verify the bot is running** – Check logs or use the Telegram bot commands.

You can now switch back to any of the earlier methods (Docker Compose, Systemd, PM2) for more robust process management.

---
#### Fedora VPS Host Tutorial

Deploy the bot on a Fedora (38/39) VPS.

1. **Provision a VPS** – Choose any provider and create a server with at least 1 CPU and 1 GB RAM.
2. **SSH into the server**:
   ```bash
   ssh root@your-vps-ip
   ```
3. **Update the system**:
   ```bash
   dnf update -y
   ```
4. **Install required packages** – Go, Docker, Git, and firewalld:
   ```bash
   # Install Go (1.24+)
   dnf install -y golang
   # Install Docker
   dnf install -y docker docker-compose
   systemctl enable --now docker
   # Install Git
   dnf install -y git
   # Enable firewalld
   dnf install -y firewalld
   systemctl enable --now firewalld
   ```
5. **Create a non‑root user** (optional):
   ```bash
   adduser botuser
   usermod -aG docker botuser
   su - botuser
   ```
6. **Clone the repository**:
   ```bash
   git clone https://github.com/yourusername/Moviebot001-main.git
   cd Moviebot001-main
   ```
7. **Set environment variables**:
   ```bash
   cp .env.example .env
   nano .env   # edit BOT_TOKEN, ADMINS, MONGODB_URI, etc.
   ```
8. **Deploy** – Use Docker Compose (Method A) or build the binary:
   ```bash
   go build -o moviebot main.go
   ./moviebot &
   ```
9. **Configure firewall** – Allow SSH and optional health‑check port:
   ```bash
   firewall-cmd --add-service=ssh --permanent
   firewall-cmd --add-port=10000/tcp --permanent   # optional
   firewall-cmd --reload
   ```
10. **Verify** – Check Docker logs or run the bot commands in Telegram.

You can now switch to any of the earlier deployment methods (Docker Compose, Systemd, PM2) for production‑grade management.

---

### Render Deployment

This repository includes a `render.yaml` blueprint file for easy deployment as a Web Service.

#### Web Service setup (Render Free Tier)
The bot includes a built-in health check HTTP server that listens on the `PORT` environment variable (default `10001` or `10000`). This ensures Render's health check passes and the container stays active.

1. Push your code to GitHub.
2. Go to the **Render Dashboard** -> **Blueprints** -> **New Blueprint Instance**.
3. Connect your repository.
4. Render will automatically detect the `render.yaml` configuration.
5. Provide the required environment variables (`BOT_TOKEN`, `MONGODB_URI`, `ADMINS`, `APP_ID`, `APP_HASH`) in the Render Dashboard when prompted.
6. Click **Deploy**.

#### Memory Optimizations for Render Free Tier
The bot includes a memory recycler routine (`debug.FreeOSMemory()` every 5 minutes and `debug.SetGCPercent(50)`) designed specifically to keep the RAM footprint under Render's **512MB free-tier limit**.

## License

All data handled by the bot is the responsibility of the user. This project is provided as-is.
