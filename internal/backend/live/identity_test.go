package live

import (
	"testing"

	"app/internal/backend"
	coolapk "github.com/XiaoMengXinX/CoolapkApi-Go"
	"github.com/XiaoMengXinX/CoolapkApi-Go/token"
)

func TestSystemProfileKeepsTheInstallationWhenSwitchingModes(t *testing.T) {
	dir := t.TempDir()
	previous := coolapkIdentity{
		Device:      token.Device{ID: "existing-id", Model: "iPhone 15", HardwareID: "iPhone15,4", SystemVersion: "26.7", SalesModel: "old-retail-model"},
		InstallTime: 12345,
		Session:     coolapk.Session{UID: "42", CDID: "existing-id"},
	}
	if err := saveCoolapkIdentity(dir, previous); err != nil {
		t.Fatal(err)
	}
	profile := backend.DeviceProfile{Model: "iPhone 17 Pro", HardwareID: "iPhone18,1", SystemVersion: "27.0.1"}
	real, err := loadCoolapkIdentity(dir, profile)
	if err != nil {
		t.Fatal(err)
	}
	if real.Device.Model != profile.Model || real.Device.HardwareID != profile.HardwareID || real.Device.SystemVersion != profile.SystemVersion ||
		real.Device.SalesModel != "" || real.Device.ID != previous.Device.ID || real.InstallTime != previous.InstallTime || real.Session != previous.Session {
		t.Fatalf("unexpected system identity: %+v", real)
	}
	random, err := loadCoolapkIdentity(dir, backend.DeviceProfile{})
	if err != nil {
		t.Fatal(err)
	}
	if random.SystemProfile || random.Device.SalesModel == "" || random.Device.ID != previous.Device.ID || random.Session != previous.Session {
		t.Fatalf("unexpected random identity: %+v", random)
	}
}
