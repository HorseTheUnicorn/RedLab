package server

import (
	"encoding/json"
	"testing"
)

func FuzzTerminalEnvelope(f *testing.F) {
	f.Add([]byte(`{"type":"input","sequence":1,"data":"id"}`))
	f.Add([]byte(`{"type":"ack","sequence":12}`))
	f.Add([]byte(`{"type":"output","stdout":"ok\\n","exitCode":0}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		var message terminalMessage
		_ = json.Unmarshal(data, &message)
		if len(message.Data) > 1<<20 || len(message.Stdout) > 1<<20 || len(message.Stderr) > 1<<20 {
			t.Skip()
		}
	})
}
