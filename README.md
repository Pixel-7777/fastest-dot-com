# fastest[dot]com

### 1. Installation
Requires GO
You must have [Go installed](https://go.dev/doc/install) on your system (version 1.20 or higher recommended).

Requires Linux or [build libpcap on Windows](https://github.com/the-tcpdump-group/libpcap/blob/master/doc/README.windows.md)

### 2. Install the Packet Capture Driver
* **Windows:** You MUST install [Npcap](https://npcap.com/) (Ensure "Install Npcap in WinPcap API-compatible Mode" is checked during installation).
* **Linux:** Install `libpcap`:

###  If you are developing or running this project inside Visual Studio Code, follow these steps:
1. Open terminal
2. Clone the Repository
```bash
   git clone https://github.com/Pixel-7777/fastest-dot-com.git
   cd fastest-dot-com
```
3. Install Dependencies(Only for windows)
```bash
    go mod tidy
```
4. Run the Application

(1) For Windows
```bash
    go run main.go
```
(2) For Linux
```bash
    sudo go run main.go
```