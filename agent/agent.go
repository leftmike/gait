package agent

import (
	"context"

	"github.com/leftmike/sandbox"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/mcpclient"
	"github.com/leftmike/gait/model"
	"github.com/leftmike/gait/skill"
	"github.com/leftmike/gait/tool"
)

type Agent struct {
	Client        model.Client
	Model         model.Model
	ModelConfig   config.ModelConfig
	Tools         map[string]tool.Tool
	Sandbox       *sandbox.Sandbox
	SandboxConfig *config.SandboxConfig

	Skills []*skill.Skill
	clnts  []*mcpclient.Client // XXX: rename field
}

func (ag *Agent) AddServer(ctx context.Context, svrCfg config.MCPServer, verbose bool) error {
	clnt, err := mcpclient.NewClient(ctx, svrCfg, verbose)
	if err != nil {
		return err
	}
	clnt.AddTools(ag.Tools)
	ag.clnts = append(ag.clnts, clnt)
	return nil
}

func (ag *Agent) AddSkill(dir string) error {
	skills, err := skill.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, sk := range skills {
		// XXX: ag.fs.AddTree(sk.Dir, false)
		ag.Skills = append(ag.Skills, sk)
	}

	return nil
}

func (ag *Agent) SystemPrompt(st model.State) {
	if len(ag.Skills) > 0 {
		st.SystemPrompt(skill.SystemPrompt(ag.Skills))
	}
}
