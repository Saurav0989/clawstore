package launchd

import (
	"fmt"
	"strings"
)

const baseTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.saurav0989.clawstore</string>
    <key>ProgramArguments</key>
    <array>
        <string>__BINARY__</string>
        <string>serve</string>
        <string>--port</string>
        <string>__PORT__</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>__HOME__/.clawstore/daemon.log</string>
    <key>StandardErrorPath</key>
    <string>__HOME__/.clawstore/daemon.error.log</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>HOME</key>
        <string>__HOME__</string>
        <key>OLLAMA_HOST</key>
        <string>__OLLAMA_HOST__</string>
    </dict>
</dict>
</plist>
`

func RenderPlist(home, binaryPath string, port int, ollamaHost string) string {
	out := baseTemplate
	out = strings.ReplaceAll(out, "__HOME__", home)
	out = strings.ReplaceAll(out, "__BINARY__", binaryPath)
	out = strings.ReplaceAll(out, "__PORT__", fmt.Sprintf("%d", port))
	out = strings.ReplaceAll(out, "__OLLAMA_HOST__", strings.TrimSpace(ollamaHost))
	return out
}
