package skill

import "context"

type Registry struct{ skills []Skill }

func NewRegistry() *Registry         { return &Registry{skills: []Skill{}} }
func (r *Registry) Register(s Skill) { r.skills = append(r.skills, s) }
func (r *Registry) Query(ctx context.Context, query Query) (Result, error) {
	result := Result{}
	for _, field := range query.Fields {
		handled := false
		for _, s := range r.skills {
			if !s.Supports(field) {
				continue
			}
			handled = true
			item, err := s.Execute(ctx, query)
			if err != nil {
				result.Issues = append(result.Issues, s.Name()+": "+err.Error())
				continue
			}
			result.Observations = append(result.Observations, item.Observations...)
			result.Issues = append(result.Issues, item.Issues...)
		}
		if !handled {
			result.Issues = append(result.Issues, "no skill registered for field: "+field)
		}
	}
	return result, nil
}
