#!/bin/bash

set -e # Exit immediately if a command exits with a non-zero status

echo "🚀 Installing fastest-dot-com..."

# 1. Check for and install libpcap-dev (Currently tailored for Debian/Ubuntu/Mint)
if [ -x "$(command -v apt-get)" ]; then
    echo "📦 Installing libpcap dependency..."
    sudo apt-get update -qq
    sudo apt-get install -y libpcap-dev
else
    echo "⚠️  Could not find apt-get. Please ensure libpcap is installed manually."
fi

# 2. Clone the repository to a temporary directory
echo "📥 Downloading source code..."
rm -rf /tmp/fastest-dot-com-build
git clone -q https://github.com/Pixel-7777/fastest-dot-com.git /tmp/fastest-dot-com-build
cd /tmp/fastest-dot-com-build

# 3. Build the Go binary
echo "🔨 Compiling the application..."
if ! command -v go &> /dev/null; then
    echo "❌ Error: Go is not installed. Please install Go and try again."
    exit 1
fi
go build -o fastest-dot-com main.go

# 4. Install globally and set network permissions so sudo isn't needed
echo "⚙️  Setting up global command and permissions..."
sudo mv fastest-dot-com /usr/local/bin/
# This magic line lets the binary capture packets WITHOUT needing sudo every time!
sudo setcap cap_net_raw,cap_net_admin=eip /usr/local/bin/fastest-dot-com

# 5. Clean up
echo "🧹 Cleaning up..."
rm -rf /tmp/fastest-dot-com-build

echo ""
echo "✅ Success! You can now run the app from anywhere by typing:"
echo "   fastest-dot-com"