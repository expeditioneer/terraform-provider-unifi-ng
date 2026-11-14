package datasources

import (
	"encoding/json"
	"testing"
)

func TestDeviceDecodesSpecFields(t *testing.T) {
	var dev deviceAPIItem
	raw := `{"id":"1","macAddress":"94:2a:6f:26:c6:ca","ipAddress":"192.168.1.55","name":"IW HD","model":"UHDIW","state":"ONLINE","firmwareVersion":"6.6.55"}`
	if err := json.Unmarshal([]byte(raw), &dev); err != nil {
		t.Fatal(err)
	}
	want := deviceAPIItem{FirmwareVersion: "6.6.55", ID: "1", IPAddress: "192.168.1.55", MAC: "94:2a:6f:26:c6:ca", Model: "UHDIW", Name: "IW HD", State: "ONLINE"}
	if dev != want {
		t.Errorf("got %+v, want %+v", dev, want)
	}
}
