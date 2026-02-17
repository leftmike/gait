package agent

import (
	"context"
	"encoding/json"

	"github.com/leftmike/gait/config"
	"github.com/leftmike/gait/filesys"
	"github.com/leftmike/gait/mcpclient"
	"github.com/leftmike/gait/model"
	"github.com/leftmike/gait/skill"
)

type Agent struct {
	mdl    model.Model
	tools  map[string]model.Tool
	skills []*skill.Skill
	fs     *filesys.ForestFS
	clnts  []*mcpclient.Client
}

func NewAgent(mdl model.Model) *Agent {
	return &Agent{
		mdl:   mdl,
		tools: map[string]model.Tool{},
		fs:    filesys.NewForestFS(),
	}
}

func (ag *Agent) Close() {
	ag.fs.Close()
}

func (ag *Agent) AddTool(name, desc string, fn model.ToolFunc, scm model.ToolSchema) {
	ag.tools[name] = model.Tool{
		Name:        name,
		Description: desc,
		Func:        fn,
		Schema:      scm,
	}
}

func (ag *Agent) AddServer(ctx context.Context, svrCfg config.MCPServer, verbose bool) error {
	clnt, err := mcpclient.NewClient(ctx, svrCfg, verbose)
	if err != nil {
		return err
	}
	clnt.AddTools(ag.tools)
	ag.clnts = append(ag.clnts, clnt)
	return nil
}

func (ag *Agent) AddSkill(dir string) error {
	skills, err := skill.ReadDir(dir)
	if err != nil {
		return err
	}

	ag.AddReadFileTool()
	// XXX: ag.AddListFilesTool()

	for _, sk := range skills {
		ag.fs.AddTree(sk.Dir, false)
		ag.skills = append(ag.skills, sk)
	}

	return nil
}

func (ag *Agent) Skills() []*skill.Skill {
	return ag.skills
}

func (ag *Agent) SystemPrompt(st model.State) {
	if len(ag.skills) > 0 {
		st.SystemPrompt(skill.SystemPrompt(ag.skills))
	}
}

func (ag *Agent) Generate(ctx context.Context, st model.State, opts *model.Options) error {
	return ag.mdl.Generate(ctx, st, ag.tools, opts)
}

type readFileArgs struct {
	Path string `json:"path" gait:"the path of the file to read"`
}

func (ag *Agent) readFile(ctx context.Context, buf []byte) (string, error) {
	var args readFileArgs
	err := json.Unmarshal(buf, &args)
	if err != nil {
		return "", err
	}

	buf, err = ag.fs.ReadFile(args.Path)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

func (ag *Agent) AddReadFileTool() {
	if _, ok := ag.tools["read_file"]; !ok {
		ag.tools["read_file"] = model.Tool{
			Name:        "read_file",
			Description: "reads the contents of a file",
			Func:        ag.readFile,
			Schema:      model.MustToolSchema[readFileArgs](),
		}
	}
}
