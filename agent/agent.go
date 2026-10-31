package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/mcp"
	"github.com/leftmike/gait/model"
	"github.com/leftmike/gait/skill"
	"github.com/leftmike/gait/system"
	"github.com/leftmike/gait/tool"
)

type Agent struct {
	AgentConfig config.AgentConfig
	Client      model.Client
	Model       model.Model
	Tools       map[string]tool.Tool
	Sandbox     *system.Sandbox

	Skills []*skill.Skill
	clnts  []*mcp.Client // XXX: rename field
}

func (agnt *Agent) AddServer(ctx context.Context, svrCfg config.MCPServer, verbose bool) error {
	clnt, err := mcp.NewClient(ctx, svrCfg, verbose)
	if err != nil {
		return err
	}
	clnt.AddTools(agnt.Tools)
	agnt.clnts = append(agnt.clnts, clnt)
	return nil
}

func (agnt *Agent) AddSkill(dir string) error {
	skills, err := skill.ReadDir(dir)
	errs := []error{err}
	for _, sk := range skills {
		if prev := skill.FindSkill(agnt.Skills, sk.Name); prev != nil {
			errs = append(errs, fmt.Errorf("duplicate skill %s: %s ignored, using %s", sk.Name,
				sk.Dir, prev.Dir))
			continue
		}
		// XXX: agnt.fs.AddTree(sk.Dir, false)
		agnt.Skills = append(agnt.Skills, sk)
	}

	return errors.Join(errs...)
}

func (agnt *Agent) SystemPrompt(st model.State) {
	if len(agnt.Skills) > 0 {
		st.SystemPrompt(skill.SystemPrompt(agnt.Skills))
	}
}
