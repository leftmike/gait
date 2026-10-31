package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddSkillDuplicate(t *testing.T) {
	var dirs []string
	for range 2 {
		dir := t.TempDir()
		err := os.Mkdir(filepath.Join(dir, "dup-skill"), 0755)
		if err != nil {
			t.Fatal(err)
		}
		err = os.WriteFile(filepath.Join(dir, "dup-skill", "SKILL.md"),
			[]byte("---\nname: dup-skill\ndescription: dup\n---\nbody\n"), 0644)
		if err != nil {
			t.Fatal(err)
		}
		dirs = append(dirs, dir)
	}

	var agnt Agent
	err := agnt.AddSkill(dirs[0])
	if err != nil {
		t.Fatalf("AddSkill(%s) failed with %s", dirs[0], err)
	}
	err = agnt.AddSkill(dirs[1])
	if err == nil || !strings.Contains(err.Error(), "duplicate skill dup-skill") {
		t.Errorf("AddSkill(%s) got %v want duplicate error", dirs[1], err)
	}

	if len(agnt.Skills) != 1 || agnt.Skills[0].Dir != filepath.Join(dirs[0], "dup-skill") {
		t.Errorf("AddSkill() kept %v, want only first", agnt.Skills)
	}
}
