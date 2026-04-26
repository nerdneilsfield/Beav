package sysinfo

import "testing"

func TestSudoUserResolutionRequiresAllThree(t *testing.T) {
	r := SudoUserResolver{
		LookupByUID:  func(uid uint32) (string, string, error) { return "alice", "/home/alice", nil },
		LookupByName: func(name string) (uint32, string, error) { return 1000, "/home/alice", nil },
		Lstat:        func(p string) (uint32, bool, error) { return 1000, false, nil },
	}
	got, err := r.Resolve(map[string]string{"SUDO_UID": "1000", "SUDO_USER": "alice"})
	if err != nil {
		t.Fatal(err)
	}
	if got.UID != 1000 || got.Name != "alice" || got.Home != "/home/alice" {
		t.Errorf("%+v", got)
	}
}
