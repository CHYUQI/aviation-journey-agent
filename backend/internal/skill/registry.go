package skill

import "context"

// Registry 维护所有可用的数据技能。
//
// 它把"要查的字段"翻译成"调用哪个技能"：
//   - 一个技能支持多个字段时只调用一次，避免重复请求和重复报错
//   - 没有任何技能支持的字段会记录一条 issue，而不是静默跳过
type Registry struct{ skills []Skill }

func NewRegistry() *Registry { return &Registry{skills: []Skill{}} }

func (r *Registry) Register(s Skill) { r.skills = append(r.skills, s) }

func (r *Registry) Query(ctx context.Context, query Query) (Result, error) {
	result := Result{}
	called := map[string]bool{}

	for _, field := range query.Fields {
		handled := false

		for _, s := range r.skills {
			if !s.Supports(field) {
				continue
			}
			handled = true

			if called[s.Name()] {
				continue
			}
			called[s.Name()] = true

			item, err := s.Execute(ctx, query)
			if err != nil {
				result.Issues = append(result.Issues, s.Name()+": "+err.Error())
				continue
			}
			result.Observations = append(result.Observations, item.Observations...)
			result.Issues = append(result.Issues, item.Issues...)
		}

		if !handled {
			result.Issues = append(result.Issues, "没有技能可以查询字段："+field)
		}
	}

	return result, nil
}
