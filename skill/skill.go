package skill

import (
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

type skillFrontmatter struct {
	Name          string            `yaml:"name"`        // Required
	Description   string            `yaml:"description"` // Required
	License       string            `yaml:"license"`
	Compatibility string            `yaml:"compatibility"`
	AllowedTools  string            `yaml:"allowed-tools"`
	Metadata      map[string]string `yaml:"metadata"`
}

type Skill struct {
	skillFrontmatter
	Body string
	Dir  string
}

func readSkill(dir, name string) (*Skill, error) {
	dir = filepath.Join(dir, name)
	buf, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return nil, err
	}

	sf, body, err := parseSkill(name, string(buf))
	if err != nil {
		return nil, err
	}

	return &Skill{
		skillFrontmatter: sf,
		Body:             body,
		Dir:              dir,
	}, nil
}

var (
	kebabCaseRegex = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
)

func parseSkill(name, s string) (skillFrontmatter, string, error) {
	var sf skillFrontmatter

	sections := strings.Split(strings.TrimSpace(s), "---\n")
	if len(sections) != 3 || sections[0] != "" {
		return sf, "", errors.New("skill file format must be frontmatter followed by body")
	}

	err := yaml.Unmarshal([]byte(sections[1]), &sf)
	if err != nil {
		return sf, "", err
	}

	if sf.Name != name {
		return sf, "", fmt.Errorf("skill directory must match skill name: %s %s", name, sf.Name)
	} else if name != strings.ToLower(name) || !kebabCaseRegex.MatchString(name) {
		return sf, "",
			fmt.Errorf("skill name must be lowercase letters, numbers, and hyphens only: %s", name)
	} else if len(name) > 64 {
		return sf, "", fmt.Errorf("skill name longer than 64 characters: %s", name)
	} else if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") ||
		strings.Contains(name, "--") {

		return sf, "",
			fmt.Errorf("skill name must not start or end with a hyphen or contain --: %s", name)
	}

	if sf.Description == "" {
		return sf, "", fmt.Errorf("skill description is required: %s", name)
	} else if utf8.RuneCountInString(sf.Description) > 1024 {
		return sf, "", fmt.Errorf("skill description longer than 1024 characters: %s", name)
	}

	if utf8.RuneCountInString(sf.Compatibility) > 500 {
		return sf, "", fmt.Errorf("skill compatibility longer than 500 characters: %s", name)
	}

	return sf, sections[2], nil
}

func ReadDir(dir string) ([]*Skill, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var skills []*Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		sk, err := readSkill(dir, entry.Name())
		if err != nil {
			continue
		}
		skills = append(skills, sk)
	}

	return skills, nil
}

func SystemPrompt(skills []*Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var buf strings.Builder
	buf.WriteString("<available_skills>\n")
	for _, sk := range skills {
		buf.WriteString("<skill>\n")
		buf.WriteString("<name>\n")
		buf.WriteString(html.EscapeString(sk.Name))
		buf.WriteString("\n</name>\n")
		buf.WriteString("<description>\n")
		buf.WriteString(html.EscapeString(sk.Description))
		buf.WriteString("\n</description>\n")
		buf.WriteString("<location>\n")
		buf.WriteString(html.EscapeString(filepath.Join(sk.Dir, "SKILL.md")))
		buf.WriteString("\n</location>\n")
		buf.WriteString("</skill>\n")
	}
	buf.WriteString("</available_skills>")
	return buf.String()
}

func FindSkill(skills []*Skill, name string) *Skill {
	for _, sk := range skills {
		if sk.Name == name {
			return sk
		}
	}

	return nil
}
