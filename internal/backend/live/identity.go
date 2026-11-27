package live

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"app/internal/backend"
	coolapk "github.com/XiaoMengXinX/CoolapkApi-Go"
	"github.com/XiaoMengXinX/CoolapkApi-Go/token"
)

type coolapkIdentity struct {
	Device        token.Device    `json:"device"`
	InstallTime   int64           `json:"installTime"`
	Session       coolapk.Session `json:"session,omitempty"`
	SystemProfile bool            `json:"systemProfile,omitempty"`
}

func loadCoolapkIdentity(dir string, profile backend.DeviceProfile) (coolapkIdentity, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return coolapkIdentity{}, err
	}
	path := filepath.Join(dir, "identity.json")
	identity := coolapkIdentity{}
	data, readErr := os.ReadFile(path)
	if readErr != nil && !os.IsNotExist(readErr) {
		return coolapkIdentity{}, readErr
	}
	if readErr != nil || json.Unmarshal(data, &identity) != nil || !validIdentity(identity) {
		identity = coolapkIdentity{Device: token.NewMockDevice(), InstallTime: time.Now().UnixMilli()}
	} else if profile.HardwareID == "" && !identity.SystemProfile {
		return identity, nil
	}
	if profile.HardwareID != "" {
		system := token.Device{
			ID: identity.Device.ID, Model: profile.Model,
			HardwareID: profile.HardwareID, SystemVersion: profile.SystemVersion,
		}
		if identity.SystemProfile && identity.Device == system {
			return identity, nil
		}
		identity.Device = system
	} else if identity.SystemProfile {
		id := identity.Device.ID
		identity.Device = token.NewMockDevice()
		identity.Device.ID = id
	}
	identity.SystemProfile = profile.HardwareID != ""
	if err := saveCoolapkIdentity(dir, identity); err != nil {
		return coolapkIdentity{}, err
	}
	return identity, nil
}

func saveCoolapkIdentity(dir string, identity coolapkIdentity) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(identity)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "identity-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, filepath.Join(dir, "identity.json"))
}

func validIdentity(identity coolapkIdentity) bool {
	d := identity.Device
	return identity.InstallTime > 0 && d.ID != "" && d.Model != "" && d.HardwareID != "" && d.SystemVersion != ""
}

func normalizeCoolapkImage(raw string) string {
	if strings.HasPrefix(raw, "http://image.coolapk.com/") || strings.HasPrefix(raw, "http://avatar.coolapk.com/") {
		return "https://" + strings.TrimPrefix(raw, "http://")
	}
	return raw
}
