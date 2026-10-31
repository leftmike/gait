package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestParseSkill(t *testing.T) {
	cases := []struct {
		name string
		s    string
		sf   skillFrontmatter
		body string
		fail bool
	}{
		{
			name: "test-skill",
			s: `---
name: test-skill
description: a test skill for unit testing
---
# Test Skill
`,
			sf: skillFrontmatter{
				Name:        "test-skill",
				Description: "a test skill for unit testing",
			},
			body: `# Test Skill
`,
		},
		{
			s: `---
name: test-skill
description: a test skill for unit testing
# Test Skill
`,
			fail: true,
		},
		{
			name: "test-skill",
			s: `---
name: test-skill
description: a test skill for unit testing
---
# Test Skill
---
Endmatter
`,
			sf: skillFrontmatter{
				Name:        "test-skill",
				Description: "a test skill for unit testing",
			},
			body: `# Test Skill
---
Endmatter
`,
		},
		{
			name: "test-skill",
			s:    "---\r\nname: test-skill\r\ndescription: crlf\r\n---  \r\n# Test Skill\r\n",
			sf: skillFrontmatter{
				Name:        "test-skill",
				Description: "crlf",
			},
			body: "# Test Skill\r\n",
		},
		{
			name: "test-skill",
			s: `---
name: test-skill
description: "   "
---
# Test Skill
`,
			fail: true,
		},
		{
			name: "test-skill",
			s: `---
name: test-skill
description: no body
---`,
			sf: skillFrontmatter{
				Name:        "test-skill",
				Description: "no body",
			},
		},
		{
			name: "test-skill",
			s: `--- 
name: test-skill
description: a test skill for unit testing
---text
# Test Skill
`,
			fail: true,
		},
		{
			s: `Test Skill
---
name: test-skill
description: a test skill for unit testing
---
# Test Skill
`,
			fail: true,
		},
		{
			s: `name: test-skill
description: a test skill for unit testing
---
# Test Skill
`,
			fail: true,
		},
		{
			s: `name: test-skill
description: a test skill for unit testing
---
# Test Skill
---

`,
			fail: true,
		},
		{
			name: "test-skill",
			s: `---
name: test-skill
description: a test skill for unit testing
license: MIT
metadata:
    author: mike m
    version: "1.0"
---
# Test Skill
`,
			sf: skillFrontmatter{
				Name:        "test-skill",
				Description: "a test skill for unit testing",
				License:     "MIT",
				Metadata: map[string]string{
					"author":  "mike m",
					"version": "1.0",
				},
			},
			body: `# Test Skill
`,
		},
		{
			name: "test-skill",
			s: `---
name: skill-test
description: a test skill for unit testing
---
# Test Skill
`,
			fail: true,
		},
		{
			name: "Test-Skill",
			s: `---
name: Test-Skill
description: a test skill for unit testing
---
# Test Skill
`,
			fail: true,
		},
		{
			name: "test$skill",
			s: `---
name: test$skill
description: a test skill for unit testing
---
# Test Skill
`,
			fail: true,
		},
		{
			name: "1234567890123456789012345678901234567890123456789012345678901234",
			s: `---
name: 1234567890123456789012345678901234567890123456789012345678901234
description: a test skill for unit testing
---
# Test Skill
`,
			sf: skillFrontmatter{
				Name:        "1234567890123456789012345678901234567890123456789012345678901234",
				Description: "a test skill for unit testing",
			},
			body: `# Test Skill
`,
		},
		{
			name: "12345678901234567890123456789012345678901234567890123456789012345",
			s: `---
name: 12345678901234567890123456789012345678901234567890123456789012345
description: a test skill for unit testing
---
# Test Skill
`,
			fail: true,
		},
		{
			name: "-test-skill",
			s: `---
name: -test-skill
description: a test skill for unit testing
---
# Test Skill
`,
			fail: true,
		},
		{
			name: "test-skill-",
			s: `---
name: test-skill-
description: a test skill for unit testing
---
# Test Skill
`,
			fail: true,
		},
		{
			name: "test--skill",
			s: `---
name: test--skill
description: a test skill for unit testing
---
# Test Skill
`,
			fail: true,
		},
	}

	for _, c := range cases {
		sf, body, err := parseSkill(c.name, c.s)
		if err != nil {
			if !c.fail {
				t.Errorf("parseSkill(%s) failed with %s", c.s, err)
			}
			continue
		}
		if c.fail {
			t.Errorf("parseSkill(%s) did not fail", c.s)
			continue
		}
		if !reflect.DeepEqual(c.sf, sf) {
			t.Errorf("parseSkill(%s) got %v want %v", c.s, sf, c.sf)
		}
		if c.body != body {
			t.Errorf("parseSkill(%s) got %q want %q", c.s, body, c.body)
		}
	}
}

func testSkill(t *testing.T, fn string, sk *Skill, dir, name, desc, license, compat, allowed,
	body string) {

	if sk.Name != name {
		t.Errorf("%s(%s) name got %s want %s", fn, dir, sk.Name, name)
	}
	if sk.Description != desc {
		t.Errorf("%s(%s) description got %s want %s", fn, dir, sk.Description, desc)
	}
	if sk.License != license {
		t.Errorf("%s(%s) license got %s want %s", fn, dir, sk.License, license)
	}
	if sk.Compatibility != compat {
		t.Errorf("%s(%s) compatibility got %s want %s", fn, dir, sk.Compatibility, compat)
	}
	if sk.AllowedTools != allowed {
		t.Errorf("%s(%s) allowed tools got %s want %s", fn, dir, sk.AllowedTools, allowed)
	}
	if sk.Body != body {
		t.Errorf("%s(%s) body got `%s` want `%s`", fn, dir, sk.Body, body)
	}
	if sk.Dir != dir {
		t.Errorf("%s(%s) name got %s want %s", fn, dir, sk.Dir, dir)
	}
}

func testReadSkill(t *testing.T, tempDir, name, desc, license, compat, allowed,
	body string) *Skill {

	dir := filepath.Join(tempDir, name)
	sk, err := readSkill(tempDir, name)
	if err != nil {
		t.Errorf("readSkill(%s) failed with %s", dir, err)
		return nil
	}

	testSkill(t, "readSkill", sk, dir, name, desc, license, compat, allowed, body)
	return sk
}

func TestRead(t *testing.T) {
	name := "test-skill"
	desc := "A test skill for unit testing"
	license := "MIT"
	compat := "Go 1.21+"
	allowed := "Bash(go:*)"
	author := "Test Author"
	version := "1.0"
	body := `# Test Skill

This is the body of the skill.

## Instructions

Follow these steps.`
	s := fmt.Sprintf(`---
name: %s
description: %s
license: %s
compatibility: %s
allowed-tools: %s
metadata:
  author: %s
  version: %s
---
%s`, name, desc, license, compat, allowed, author, version, body)

	tempDir := t.TempDir()
	dir := filepath.Join(tempDir, name)
	err := os.Mkdir(dir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(s), 0644)
	if err != nil {
		t.Fatal(err)
	}

	sk := testReadSkill(t, tempDir, name, desc, license, compat, allowed, body)
	if sk != nil {
		if sk.Metadata["author"] != author {
			t.Errorf("readSkill(%s) author got %s want %s", dir, sk.Metadata["author"], author)
		}
		if sk.Metadata["version"] != version {
			t.Errorf("readSkill(%s) version got %s want %s", dir, sk.Metadata["version"], version)
		}
	}
}

func TestReadDir(t *testing.T) {
	tempDir := t.TempDir()
	names := []string{"test-skill", "skill-one", "skill-two"}

	for _, name := range names {
		dir := filepath.Join(tempDir, name)
		err := os.Mkdir(dir, 0755)
		if err != nil {
			t.Fatal(err)
		}

		s := fmt.Sprintf(`---
name: %s
description: Skill %s
---
%s`, name, name, name)

		err = os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(s), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	err := os.Mkdir(filepath.Join(tempDir, "not-a-skill"), 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(tempDir, "not-a-skill", "AGENTS.md"), []byte("nothing"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.WriteFile(filepath.Join(tempDir, "README.md"), []byte("nothing"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	skills, err := ReadDir(tempDir)
	if err != nil {
		t.Errorf("ReadDir() failed with %s", err)
	}
	if len(skills) != len(names) {
		t.Errorf("ReadDir() returned %d skills, want %d", len(skills), len(names))
	}

	for _, sk := range skills {
		testSkill(t, "ReadDir", sk, filepath.Join(tempDir, sk.Name), sk.Name, "Skill "+sk.Name,
			"", "", "", sk.Name)
		if !slices.Contains(names, sk.Name) {
			t.Errorf("ReadDir() unexpected skill name %s", sk.Name)
		}
	}
}

func TestReadDirErrors(t *testing.T) {
	tempDir := t.TempDir()
	write := func(dir, s string) {
		err := os.MkdirAll(filepath.Join(tempDir, dir), 0755)
		if err != nil {
			t.Fatal(err)
		}
		err = os.WriteFile(filepath.Join(tempDir, dir, "SKILL.md"), []byte(s), 0644)
		if err != nil {
			t.Fatal(err)
		}
	}

	write("good-skill", "---\nname: good-skill\ndescription: good\n---\nbody\n")
	write("wrong-name", "---\nname: other-name\ndescription: bad\n---\nbody\n")
	write("no-description", "---\nname: no-description\n---\nbody\n")

	target := t.TempDir()
	write("../"+filepath.Base(target)+"/linked-skill",
		"---\nname: linked-skill\ndescription: linked\n---\nbody\n")
	err := os.Symlink(filepath.Join(target, "linked-skill"), filepath.Join(tempDir, "linked-skill"))
	if err != nil {
		t.Fatal(err)
	}

	skills, err := ReadDir(tempDir)
	var got []string
	for _, sk := range skills {
		got = append(got, sk.Name)
	}
	slices.Sort(got)
	if want := []string{"good-skill", "linked-skill"}; !slices.Equal(got, want) {
		t.Errorf("ReadDir() got skills %v want %v", got, want)
	}

	if err == nil {
		t.Fatal("ReadDir() did not return errors for invalid skills")
	}
	for _, dir := range []string{"wrong-name", "no-description"} {
		if !strings.Contains(err.Error(), filepath.Join(tempDir, dir)) {
			t.Errorf("ReadDir() error missing %s: %s", dir, err)
		}
	}
}

func TestSystemPrompt(t *testing.T) {
	skills := []*Skill{
		{
			skillFrontmatter: skillFrontmatter{
				Name:        "test-skill",
				Description: "A test skill",
			},
			Dir: "/home/user/skills/test-skill",
		},
		{
			skillFrontmatter: skillFrontmatter{
				Name:        "another-skill",
				Description: "Another <test> skill & it's \"quoted\"",
			},
			Dir: "/home/user/skills/another-skill",
		},
	}

	s := SystemPrompt(skills)

	if !strings.Contains(s, "<available_skills>") {
		t.Error("Prompt() missing <available_skills> tag")
	}
	if !strings.Contains(s, "</available_skills>") {
		t.Error("Prompt() missing </available_skills> tag")
	}
	if !strings.Contains(s, "test-skill") {
		t.Error("Prompt() missing skill name")
	}
	if !strings.Contains(s, "A test skill") {
		t.Error("Prompt() missing skill description")
	}
	if !strings.Contains(s, "/home/user/skills/test-skill/SKILL.md") {
		t.Error("Prompt() missing skill location")
	}
	if !strings.Contains(s, `Another &lt;test&gt; skill &amp; it's "quoted"`) {
		t.Errorf("Prompt() should escape only &, <, and >: %s", s)
	}
}

func TestSystemPromptEmpty(t *testing.T) {
	s := SystemPrompt(nil)
	if s != "" {
		t.Errorf(`Prompt(nil) got %q want ""`, s)
	}

	s = SystemPrompt([]*Skill{})
	if s != "" {
		t.Errorf(`Prompt([]) got %q want ""`, s)
	}
}
