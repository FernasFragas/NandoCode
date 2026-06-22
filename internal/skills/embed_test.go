package skills

import "testing"

func TestLoadBundledSkills(t *testing.T) {
	t.Parallel()
	list, err := loadBundledSkills(BundledFS)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 3 {
		t.Fatalf("expected >=3 skills, got %d", len(list))
	}
	want := map[string]bool{"code-review": false, "debug-session": false, "write-tests": false}
	for _, s := range list {
		if _, ok := want[s.Name]; ok {
			want[s.Name] = true
		}
	}
	for k, ok := range want {
		if !ok {
			t.Fatalf("missing bundled skill %s", k)
		}
	}
}
