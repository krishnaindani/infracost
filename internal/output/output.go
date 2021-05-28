package output

import (
	"fmt"
	"sort"
	"time"

	"github.com/infracost/infracost/internal/providers/terraform"
	"github.com/infracost/infracost/internal/schema"
	"github.com/shopspring/decimal"
)

var outputVersion = "0.1"

type Root struct {
	Version          string           `json:"version"`
	Resources        []Resource       `json:"resources"`        // Keeping for backward compatibility.
	TotalHourlyCost  *decimal.Decimal `json:"totalHourlyCost"`  // Keeping for backward compatibility.
	TotalMonthlyCost *decimal.Decimal `json:"totalMonthlyCost"` // Keeping for backward compatibility.
	RunID            string           `json:"runId,omitempty"`
	ProjectResults   []ProjectResult  `json:"projectResults"`
	TimeGenerated    time.Time        `json:"timeGenerated"`
}

type ProjectResult struct {
	ProjectName     string                  `json:"projectName"`
	ProjectMetadata *schema.ProjectMetadata `json:"projectMetadata"`
	PastBreakdown   *Breakdown              `json:"pastBreakdown"`
	Breakdown       *Breakdown              `json:"breakdown"`
	Diff            *Breakdown              `json:"diff"`
	Summary         *Summary                `json:"summary"`
	fullSummary     *Summary
}

type Breakdown struct {
	Resources        []Resource       `json:"resources"`
	TotalHourlyCost  *decimal.Decimal `json:"totalHourlyCost"`
	TotalMonthlyCost *decimal.Decimal `json:"totalMonthlyCost"`
}

type CostComponent struct {
	Name            string           `json:"name"`
	Unit            string           `json:"unit"`
	HourlyQuantity  *decimal.Decimal `json:"hourlyQuantity"`
	MonthlyQuantity *decimal.Decimal `json:"monthlyQuantity"`
	Price           decimal.Decimal  `json:"price"`
	HourlyCost      *decimal.Decimal `json:"hourlyCost"`
	MonthlyCost     *decimal.Decimal `json:"monthlyCost"`
}

type Resource struct {
	Name           string            `json:"name"`
	Tags           map[string]string `json:"tags,omitempty"`
	Metadata       map[string]string `json:"metadata"`
	HourlyCost     *decimal.Decimal  `json:"hourlyCost"`
	MonthlyCost    *decimal.Decimal  `json:"monthlyCost"`
	CostComponents []CostComponent   `json:"costComponents,omitempty"`
	SubResources   []Resource        `json:"subresources,omitempty"`
}

type Summary struct {
	SupportedResourceCounts   *map[string]int `json:"supportedResourceCounts,omitempty"`
	UnsupportedResourceCounts *map[string]int `json:"unsupportedResourceCounts,omitempty"`
	TotalSupportedResources   *int            `json:"totalSupportedResources,omitempty"`
	TotalUnsupportedResources *int            `json:"totalUnsupportedResources,omitempty"`
	TotalNoPriceResources     *int            `json:"totalNoPriceResources,omitempty"`
	TotalResources            *int            `json:"totalResources,omitempty"`
}

type SummaryOptions struct {
	IncludeUnsupportedProviders bool
	OnlyFields                  []string
}

type Options struct {
	NoColor     bool
	ShowSkipped bool
	GroupLabel  string
	GroupKey    string
	Fields      []string
}

func outputBreakdown(resources []*schema.Resource) *Breakdown {
	arr := make([]Resource, 0, len(resources))

	for _, r := range resources {
		if r.IsSkipped {
			continue
		}
		arr = append(arr, outputResource(r))
	}

	sortResources(arr, "")

	totalMonthlyCost, totalHourlyCost := calculateTotalCosts(arr)

	return &Breakdown{
		Resources:        arr,
		TotalHourlyCost:  totalMonthlyCost,
		TotalMonthlyCost: totalHourlyCost,
	}
}

func outputResource(r *schema.Resource) Resource {
	comps := make([]CostComponent, 0, len(r.CostComponents))
	for _, c := range r.CostComponents {

		comps = append(comps, CostComponent{
			Name:            c.Name,
			Unit:            c.UnitWithMultiplier(),
			HourlyQuantity:  c.UnitMultiplierHourlyQuantity(),
			MonthlyQuantity: c.UnitMultiplierMonthlyQuantity(),
			Price:           c.UnitMultiplierPrice(),
			HourlyCost:      c.HourlyCost,
			MonthlyCost:     c.MonthlyCost,
		})
	}

	subresources := make([]Resource, 0, len(r.SubResources))
	for _, s := range r.SubResources {
		subresources = append(subresources, outputResource(s))
	}

	return Resource{
		Name:           r.Name,
		Metadata:       map[string]string{},
		Tags:           r.Tags,
		HourlyCost:     r.HourlyCost,
		MonthlyCost:    r.MonthlyCost,
		CostComponents: comps,
		SubResources:   subresources,
	}
}

func ToOutputFormat(projects []*schema.Project) Root {
	var totalMonthlyCost, totalHourlyCost *decimal.Decimal

	outProjectResults := make([]ProjectResult, 0, len(projects))
	outResources := make([]Resource, 0)

	for _, project := range projects {
		var pastBreakdown, breakdown, diff *Breakdown

		breakdown = outputBreakdown(project.Resources)

		if project.HasDiff {
			pastBreakdown = outputBreakdown(project.PastResources)
			diff = outputBreakdown(project.Diff)
		}

		// Backward compatibility
		if breakdown != nil {
			outResources = append(outResources, breakdown.Resources...)
		}

		if breakdown != nil && breakdown.TotalHourlyCost != nil {
			if totalHourlyCost == nil {
				totalHourlyCost = decimalPtr(decimal.Zero)
			}

			totalHourlyCost = decimalPtr(totalHourlyCost.Add(*breakdown.TotalHourlyCost))
		}

		if breakdown != nil && breakdown.TotalMonthlyCost != nil {
			if totalMonthlyCost == nil {
				totalMonthlyCost = decimalPtr(decimal.Zero)
			}

			totalMonthlyCost = decimalPtr(totalMonthlyCost.Add(*breakdown.TotalMonthlyCost))
		}

		summary := BuildSummary(project.Resources, SummaryOptions{
			OnlyFields: []string{"UnsupportedResourceCounts"},
		})

		fullSummary := BuildSummary(project.Resources, SummaryOptions{IncludeUnsupportedProviders: true})

		outProjectResults = append(outProjectResults, ProjectResult{
			ProjectName:     project.Name,
			ProjectMetadata: project.Metadata,
			PastBreakdown:   pastBreakdown,
			Breakdown:       breakdown,
			Diff:            diff,
			Summary:         summary,
			fullSummary:     fullSummary,
		})
	}

	sortResources(outResources, "")

	out := Root{
		Version:          outputVersion,
		Resources:        outResources,
		TotalHourlyCost:  totalHourlyCost,
		TotalMonthlyCost: totalMonthlyCost,
		ProjectResults:   outProjectResults,
		TimeGenerated:    time.Now(),
	}

	return out
}

func (r *Root) MergedSummary() *Summary {
	summaries := make([]*Summary, 0, len(r.ProjectResults))
	for _, projectResult := range r.ProjectResults {
		summaries = append(summaries, projectResult.Summary)
	}

	return MergeSummaries(summaries)
}

func (r *Root) MergedFullSummary() *Summary {
	summaries := make([]*Summary, 0, len(r.ProjectResults))
	for _, projectResult := range r.ProjectResults {
		summaries = append(summaries, projectResult.fullSummary)
	}

	return MergeSummaries(summaries)
}

func (r *Root) unsupportedResourcesMessage(showSkipped bool) string {
	summary := r.MergedSummary()

	if summary.UnsupportedResourceCounts == nil || len(*summary.UnsupportedResourceCounts) == 0 {
		return ""
	}

	unsupportedTypeCount := len(*summary.UnsupportedResourceCounts)

	unsupportedMsg := "resource types weren't estimated as they're not supported yet"
	if unsupportedTypeCount == 1 {
		unsupportedMsg = "resource type wasn't estimated as it's not supported yet"
	}

	showSkippedMsg := ", rerun with --show-skipped to see"
	if showSkipped {
		showSkippedMsg = ""
	}

	msg := fmt.Sprintf("%d %s%s.\n%s",
		unsupportedTypeCount,
		unsupportedMsg,
		showSkippedMsg,
		"Please watch/star https://github.com/infracost/infracost as new resources are added regularly.",
	)

	if showSkipped {
		for t, c := range *summary.UnsupportedResourceCounts {
			msg += fmt.Sprintf("\n%d x %s", c, t)
		}
	}

	return msg
}

func BuildSummary(resources []*schema.Resource, opts SummaryOptions) *Summary {
	supportedResourceCounts := make(map[string]int)
	unsupportedResourceCounts := make(map[string]int)
	totalSupportedResources := 0
	totalUnsupportedResources := 0
	totalNoPriceResources := 0

	for _, r := range resources {
		if !opts.IncludeUnsupportedProviders && !terraform.HasSupportedProvider(r.ResourceType) {
			continue
		}

		if r.NoPrice {
			totalNoPriceResources++
		} else if r.IsSkipped {
			totalUnsupportedResources++
			if _, ok := unsupportedResourceCounts[r.ResourceType]; !ok {
				unsupportedResourceCounts[r.ResourceType] = 0
			}
			unsupportedResourceCounts[r.ResourceType]++
		} else {
			totalSupportedResources++
			if _, ok := supportedResourceCounts[r.ResourceType]; !ok {
				supportedResourceCounts[r.ResourceType] = 0
			}
			supportedResourceCounts[r.ResourceType]++
		}
	}

	totalResources := len(resources)

	s := &Summary{}

	if len(opts.OnlyFields) == 0 || contains(opts.OnlyFields, "SupportedResourceCounts") {
		s.SupportedResourceCounts = &supportedResourceCounts
	}
	if len(opts.OnlyFields) == 0 || contains(opts.OnlyFields, "UnsupportedResourceCounts") {
		s.UnsupportedResourceCounts = &unsupportedResourceCounts
	}
	if len(opts.OnlyFields) == 0 || contains(opts.OnlyFields, "TotalSupportedResources") {
		s.TotalSupportedResources = &totalSupportedResources
	}
	if len(opts.OnlyFields) == 0 || contains(opts.OnlyFields, "TotalUnsupportedResources") {
		s.TotalUnsupportedResources = &totalUnsupportedResources
	}
	if len(opts.OnlyFields) == 0 || contains(opts.OnlyFields, "TotalNoPriceResources") {
		s.TotalNoPriceResources = &totalNoPriceResources
	}
	if len(opts.OnlyFields) == 0 || contains(opts.OnlyFields, "Total") {
		s.TotalResources = &totalResources
	}

	return s
}

func MergeSummaries(summaries []*Summary) *Summary {
	merged := &Summary{}

	for _, s := range summaries {
		if s == nil {
			continue
		}

		merged.SupportedResourceCounts = mergeCounts(merged.SupportedResourceCounts, s.SupportedResourceCounts)
		merged.UnsupportedResourceCounts = mergeCounts(merged.UnsupportedResourceCounts, s.UnsupportedResourceCounts)
		merged.TotalSupportedResources = addIntPtrs(merged.TotalSupportedResources, s.TotalSupportedResources)
		merged.TotalUnsupportedResources = addIntPtrs(merged.TotalUnsupportedResources, s.TotalUnsupportedResources)
		merged.TotalNoPriceResources = addIntPtrs(merged.TotalNoPriceResources, s.TotalNoPriceResources)
		merged.TotalResources = addIntPtrs(merged.TotalResources, s.TotalResources)
	}

	return merged
}

func calculateTotalCosts(resources []Resource) (*decimal.Decimal, *decimal.Decimal) {
	totalHourlyCost := decimalPtr(decimal.Zero)
	totalMonthlyCost := decimalPtr(decimal.Zero)

	for _, r := range resources {
		if r.HourlyCost != nil {
			if totalHourlyCost == nil {
				totalHourlyCost = decimalPtr(decimal.Zero)
			}

			totalHourlyCost = decimalPtr(totalHourlyCost.Add(*r.HourlyCost))
		}
		if r.MonthlyCost != nil {
			if totalMonthlyCost == nil {
				totalMonthlyCost = decimalPtr(decimal.Zero)
			}

			totalMonthlyCost = decimalPtr(totalMonthlyCost.Add(*r.MonthlyCost))
		}

	}

	return totalHourlyCost, totalMonthlyCost
}

func sortResources(resources []Resource, groupKey string) {
	sort.Slice(resources, func(i, j int) bool {
		// If an empty group key is passed just sort by name
		if groupKey == "" {
			return resources[i].Name < resources[j].Name
		}

		// If the resources are in the same group then sort by name
		if resources[i].Metadata[groupKey] == resources[j].Metadata[groupKey] {
			return resources[i].Name < resources[j].Name
		}

		// Sort by the group key
		return resources[i].Metadata[groupKey] < resources[j].Metadata[groupKey]
	})
}

func contains(arr []string, e string) bool {
	for _, a := range arr {
		if a == e {
			return true
		}
	}
	return false
}

func decimalPtr(d decimal.Decimal) *decimal.Decimal {
	return &d
}

func breakdownHasNilCosts(breakdown Breakdown) bool {
	for _, resource := range breakdown.Resources {
		if resourceHasNilCosts(resource) {
			return true
		}
	}

	return false
}

func resourceHasNilCosts(resource Resource) bool {
	if resource.MonthlyCost == nil {
		return true
	}

	for _, costComponent := range resource.CostComponents {
		if costComponent.MonthlyCost == nil {
			return true
		}
	}

	for _, subResource := range resource.SubResources {
		if resourceHasNilCosts(subResource) {
			return true
		}
	}

	return false
}

func mergeCounts(c1 *map[string]int, c2 *map[string]int) *map[string]int {
	if c1 == nil && c2 == nil {
		return nil
	}

	res := make(map[string]int)

	if c1 != nil {
		for k, v := range *c1 {
			res[k] = v
		}
	}

	if c2 != nil {
		for k, v := range *c2 {
			res[k] += v
		}
	}

	return &res
}

func addIntPtrs(i1 *int, i2 *int) *int {
	if i1 == nil && i2 == nil {
		return nil
	}

	val1 := 0
	if i1 != nil {
		val1 = *i1
	}

	val2 := 0
	if i2 != nil {
		val2 = *i2
	}

	res := val1 + val2
	return &res
}
