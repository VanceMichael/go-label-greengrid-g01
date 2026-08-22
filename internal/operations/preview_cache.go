package operations

import "time"

func (p *Planner) cachedPreview(tenantID, clusterID string, gpu int, start, end time.Time) (Plan, bool) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()
	cached := p.lastPlan
	if cached.TenantID == tenantID && cached.ClusterID == clusterID && cached.GPU == gpu && cached.StartsAt.Equal(start.UTC()) && cached.EndsAt.Equal(end.UTC()) {
		return cached, true
	}
	return Plan{}, false
}

func (p *Planner) rememberPreview(plan Plan) {
	p.cacheMu.Lock()
	p.lastPlan = plan
	p.cacheMu.Unlock()
}
