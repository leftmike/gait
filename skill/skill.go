package skill

import (
	"errors"
	"fmt"
	"io/fs"
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
	kebabCaseRegex  = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	frontmatterLine = regexp.MustCompile(`(?m)^---[ \t]*\r?$`)
)

func splitFrontmatter(s string) (string, string, bool) {
	loc := frontmatterLine.FindStringIndex(s)
	if loc == nil || loc[0] != 0 {
		return "", "", false
	}
	rest := strings.TrimPrefix(s[loc[1]:], "\n")

	loc = frontmatterLine.FindStringIndex(rest)
	if loc == nil {
		return "", "", false
	}
	return rest[:loc[0]], strings.TrimPrefix(rest[loc[1]:], "\n"), true
}

func parseSkill(name, s string) (skillFrontmatter, string, error) {
	var sf skillFrontmatter

	front, body, ok := splitFrontmatter(strings.TrimLeft(s, "\ufeff \t\r\n"))
	if !ok {
		return sf, "", errors.New("skill file format must be frontmatter followed by body")
	}

	err := yaml.Unmarshal([]byte(front), &sf)
	if err != nil {
		return sf, "", err
	}

	if len(sf.Name) > 64 {
		return sf, "", fmt.Errorf("skill name longer than 64 characters: %s", sf.Name)
	} else if !kebabCaseRegex.MatchString(sf.Name) {
		return sf, "", fmt.Errorf(
			"skill name must be lowercase letters, numbers, and single hyphens only: %s", sf.Name)
	} else if sf.Name != name {
		return sf, "", fmt.Errorf("skill directory must match skill name: %s %s", name, sf.Name)
	}

	if strings.TrimSpace(sf.Description) == "" {
		return sf, "", fmt.Errorf("skill description is required: %s", name)
	} else if utf8.RuneCountInString(sf.Description) > 1024 {
		return sf, "", fmt.Errorf("skill description longer than 1024 characters: %s", name)
	}

	if utf8.RuneCountInString(sf.Compatibility) > 500 {
		return sf, "", fmt.Errorf("skill compatibility longer than 500 characters: %s", name)
	}

	return sf, body, nil
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
	var errs []error
	for _, entry := range entries {
		fi, err := os.Stat(filepath.Join(dir, entry.Name()))
		if err != nil || !fi.IsDir() {
			continue
		}

		sk, err := readSkill(dir, entry.Name())
		if errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", filepath.Join(dir, entry.Name()), err))
			continue
		}
		skills = append(skills, sk)
	}

	return skills, errors.Join(errs...)
}

var promptEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func SystemPrompt(skills []*Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var buf strings.Builder
	buf.WriteString("<available_skills>\n")
	for _, sk := range skills {
		buf.WriteString("<skill>\n")
		buf.WriteString("<name>\n")
		buf.WriteString(promptEscaper.Replace(sk.Name))
		buf.WriteString("\n</name>\n")
		buf.WriteString("<description>\n")
		buf.WriteString(promptEscaper.Replace(sk.Description))
		buf.WriteString("\n</description>\n")
		buf.WriteString("<location>\n")
		buf.WriteString(promptEscaper.Replace(filepath.Join(sk.Dir, "SKILL.md")))
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
