package prompts

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rs/zerolog/log"
)

func RegisterYTD(s *mcp.Server, db Database) {
	s.AddPrompt(&mcp.Prompt{
		Name:        "ytd",
		Description: "Year-to-date astrophotography statistics and summary",
		Arguments: []*mcp.PromptArgument{
			{
				Name:        "year",
				Description: "Year to analyze (default: current year)",
				Required:    false,
			},
			{
				Name:        "breakdown",
				Description: "Type of breakdown: monthly, target, filter, or equipment (default: overview)",
				Required:    false,
			},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {

		log.Info().Str("prompt", "ytd").Interface("args", req.Params.Arguments).Msg("Prompt called")

		// Parse arguments
		year := time.Now().Year()
		if yearArg, ok := req.Params.Arguments["year"]; ok && yearArg != "" {
			if y, err := fmt.Sscanf(yearArg, "%d", &year); err == nil && y == 1 {
				// Successfully parsed
			}
		}

		breakdown := "overview"
		if breakdownArg, ok := req.Params.Arguments["breakdown"]; ok && breakdownArg != "" {
			breakdown = breakdownArg
		}

		// Build the prompt message based on breakdown type
		var promptMsg string

		switch breakdown {
		case "monthly":
			promptMsg = fmt.Sprintf(`Analyze my astrophotography activity for %d with a monthly breakdown.

Please query the FITS archive and provide:

1. **Monthly Summary**:
   - Total frames captured per month
   - Total integration time per month
   - Most active month
   - Quietest month

2. **Monthly Trends**:
   - How activity changed throughout the year
   - Seasonal patterns
   - Any notable gaps or productive periods

3. **Recommendations**:
   - Best months for imaging (based on my data)
   - Suggestions for improving less productive months

Use the query_fits_archive tool to get data filtered by observation_date for each month of %d.`, year, year)

		case "target":
			promptMsg = fmt.Sprintf(`Analyze my astrophotography targets for %d.

Please query the FITS archive and provide:

1. **Target Statistics**:
   - List all targets imaged (sorted by total integration time)
   - Number of frames per target
   - Total integration time per target
   - Filters used for each target

2. **Target Analysis**:
   - Most photographed target
   - Which targets might need more data
   - Target diversity (DSO types: galaxy, nebula, etc.)

3. **Recommendations**:
   - Targets worth revisiting
   - Suggestions for new targets based on my imaging patterns

Use the query_fits_archive tool to aggregate data by object for %d.`, year, year)

		case "filter":
			promptMsg = fmt.Sprintf(`Analyze my filter usage for %d.

Please query the FITS archive and provide:

1. **Filter Statistics**:
   - Total frames per filter
   - Total integration time per filter
   - Average exposure time per filter
   - Filter usage distribution (percentage)

2. **Filter Analysis**:
   - Most used filter(s)
   - Underutilized filters
   - Filter balance for narrowband vs broadband

3. **Recommendations**:
   - Suggestions for better filter balance
   - Filters that might need more use

Use the query_fits_archive tool to aggregate data by filter for %d.`, year, year)

		case "equipment":
			promptMsg = fmt.Sprintf(`Analyze my equipment usage for %d.

Please query the FITS archive and provide:

1. **Equipment Statistics**:
   - Telescope usage breakdown
   - Camera usage breakdown
   - Most used telescope/camera combination
   - Focal length distribution

2. **Equipment Analysis**:
   - Equipment utilization patterns
   - Setup preferences (which equipment for which targets)

3. **Insights**:
   - Equipment efficiency
   - Suggestions based on usage patterns

Use the query_fits_archive tool to aggregate data by telescope and camera for %d.`, year, year)

		default: // overview
			promptMsg = fmt.Sprintf(`Provide a comprehensive year-to-date astrophotography summary for %d.

Please query the FITS archive and create a detailed report including:

1. **Overall Statistics**:
   - Total imaging sessions (count distinct observation dates)
   - Total frames captured
   - Total integration time (hours)
   - Date range (first to last observation)

2. **Top Targets** (top 5):
   - Target name
   - Number of frames
   - Total integration time
   - Filters used

3. **Filter Breakdown**:
   - Frames per filter type
   - Integration time per filter
   - Filter usage percentage

4. **Equipment Usage**:
   - Telescopes used
   - Cameras used
   - Most common setup

5. **Temporal Analysis**:
   - Most productive month
   - Average frames per session
   - Imaging frequency

6. **Highlights & Insights**:
   - Notable achievements
   - Patterns or trends
   - Suggestions for improvement

Use the query_fits_archive and get_archive_summary tools to gather comprehensive data for %d.
Format the response in a clear, structured way with markdown formatting.`, year, year)
		}

		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Year-to-date analysis for %d (%s breakdown)", year, breakdown),
			Messages: []*mcp.PromptMessage{
				{
					Role: "user",
					Content: &mcp.TextContent{
						Text: promptMsg,
					},
				},
			},
		}, nil
	})
}
