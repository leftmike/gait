package agent

import (
	"context"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/mcpclient"
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
	clnts  []*mcpclient.Client // XXX: rename field
}

func (agnt *Agent) AddServer(ctx context.Context, svrCfg config.MCPServer, verbose bool) error {
	clnt, err := mcpclient.NewClient(ctx, svrCfg, verbose)
	if err != nil {
		return err
	}
	clnt.AddTools(agnt.Tools)
	agnt.clnts = append(agnt.clnts, clnt)
	return nil
}

func (agnt *Agent) AddSkill(dir string) error {
	skills, err := skill.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, sk := range skills {
		// XXX: agnt.fs.AddTree(sk.Dir, false)
		agnt.Skills = append(agnt.Skills, sk)
	}

	return nil
}

func (agnt *Agent) SystemPrompt(st model.State) {
	if len(agnt.Skills) > 0 {
		st.SystemPrompt(skill.SystemPrompt(agnt.Skills))
	}
}
