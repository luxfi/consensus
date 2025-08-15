// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/luxfi/consensus/config"
)

// runInteractive runs the consensus tool in interactive mode
func runInteractive() error {
	scanner := bufio.NewScanner(os.Stdin)

	fmt.Println("🔧 Lux Consensus Parameter Tool - Interactive Mode")
	fmt.Println("================================================")
	fmt.Println()

	// Get network characteristics
	nc := NetworkCharacteristics{}

	nc.TotalNodes = promptInt(scanner, "Total number of nodes in network", 5, 1000, 21)

	nc.ExpectedFailureRate = promptFloat(scanner,
		"Expected Byzantine node ratio (0.0-0.33)", 0.0, 0.33, 0.20)

	nc.NetworkLatencyMs = promptInt(scanner,
		"Average network latency (ms)", 1, 1000, 50)

	nc.TargetFinalityMs = promptInt(scanner,
		"Target finality time (ms)", 100, 10000, 1000)

	nc.TargetThroughputTPS = promptInt(scanner,
		"Target throughput (TPS)", 10, 100000, 1000)

	nc.IsProduction = promptBool(scanner, "Is this for production use?", true)

	// Calculate optimal parameters
	fmt.Println("\n📊 Calculating optimal parameters...")
	params, reasoning := CalculateOptimalParameters(nc)

	// Show results
	fmt.Println("\n✨ Recommended Parameters:")
	fmt.Println("==========================")
	data, _ := ToJSON(params)
	fmt.Println(string(data))

	fmt.Println("\n📝 Reasoning:")
	fmt.Println(reasoning)

	// Perform safety analysis
	fmt.Println("\n🛡️  Safety Analysis:")
	fmt.Println("===================")
	report := AnalyzeSafety(params, nc.TotalNodes)
	displaySafetyReport(report)

	// Probability analysis
	fmt.Println("\n📈 Probability Analysis:")
	fmt.Println("=======================")
	probs := AnalyzeProbabilities(params, nc.ExpectedFailureRate)
	fmt.Printf("• Safety failure probability: %.2e\n", probs.SafetyFailureProbability)
	fmt.Printf("• Liveness failure probability: %.2e\n", probs.LivenessFailureProbability)
	fmt.Printf("• Expected rounds to finality: %.1f\n", probs.ExpectedRoundsToFinality)
	fmt.Printf("• Probability of disagreement: %.2e\n", probs.ProbabilityOfDisagreement)

	// Allow customization
	if promptBool(scanner, "\nWould you like to customize these parameters?", false) {
		params = customizeParameters(scanner, params)

		// Re-run safety analysis
		fmt.Println("\n🛡️  Updated Safety Analysis:")
		report = AnalyzeSafety(params, nc.TotalNodes)
		displaySafetyReport(report)
	}

	// Save results
	if promptBool(scanner, "\nSave parameters to file?", true) {
		filename := promptString(scanner, "Output filename", "params.json")
		data, _ := ToJSON(params)
		if err := os.WriteFile(filename, data, 0644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
		fmt.Printf("✅ Parameters saved to %s\n", filename)

		// Also save detailed report
		reportFile := strings.TrimSuffix(filename, ".json") + "-report.txt"
		if err := saveDetailedReport(reportFile, nc, params, report, probs); err != nil {
			fmt.Printf("⚠️  Failed to save report: %v\n", err)
		} else {
			fmt.Printf("📄 Detailed report saved to %s\n", reportFile)
		}
	}

	return nil
}

func customizeParameters(scanner *bufio.Scanner, p *config.Parameters) *config.Parameters {
	fmt.Println("\n🔧 Parameter Customization")
	fmt.Println("========================")
	fmt.Println("Press Enter to keep current value")

	p.K = promptIntDefault(scanner,
		fmt.Sprintf("K (sample size) [current: %d]", p.K), 1, 1000, p.K)

	p.AlphaPreference = promptIntDefault(scanner,
		fmt.Sprintf("AlphaPreference [current: %d, min: %d]", p.AlphaPreference, p.K/2+1),
		p.K/2+1, p.K, p.AlphaPreference)

	p.AlphaConfidence = promptIntDefault(scanner,
		fmt.Sprintf("AlphaConfidence [current: %d, min: %d]", p.AlphaConfidence, p.AlphaPreference),
		p.AlphaPreference, p.K, p.AlphaConfidence)

	p.Beta = uint32(promptIntDefault(scanner,
		fmt.Sprintf("Beta (rounds) [current: %d]", p.Beta), 1, 100, int(p.Beta)))

	p.ConcurrentPolls = promptIntDefault(scanner,
		fmt.Sprintf("ConcurrentPolls [current: %d, max: %d]", p.ConcurrentPolls, p.Beta),
		1, int(p.Beta), p.ConcurrentPolls)

	return p
}

func displaySafetyReport(report SafetyReport) {
	// Color codes for terminal
	colors := map[SafetyLevel]string{
		SafetyOptimal:  "\033[32m", // Green
		SafetyGood:     "\033[36m", // Cyan
		SafetyWarning:  "\033[33m", // Yellow
		SafetyCritical: "\033[31m", // Red
		SafetyDanger:   "\033[35m", // Magenta
	}
	reset := "\033[0m"

	fmt.Printf("Safety Level: %s%s%s\n", colors[report.Level], report.Level, reset)

	if len(report.Issues) > 0 {
		fmt.Println("\n❌ Critical Issues:")
		for _, issue := range report.Issues {
			fmt.Printf("   • %s\n", issue)
		}
	}

	if len(report.Warnings) > 0 {
		fmt.Println("\n⚠️  Warnings:")
		for _, warning := range report.Warnings {
			fmt.Printf("   • %s\n", warning)
		}
	}

	if len(report.Suggestions) > 0 {
		fmt.Println("\n💡 Suggestions:")
		for _, suggestion := range report.Suggestions {
			fmt.Printf("   • %s\n", suggestion)
		}
	}

	if report.Explanation != "" {
		fmt.Println("\n📖 Explanation:")
		fmt.Printf("   %s\n", report.Explanation)
	}
}

func saveDetailedReport(filename string, nc NetworkCharacteristics,
	p *config.Parameters, report SafetyReport, probs ProbabilityAnalysis) error {

	var content strings.Builder

	content.WriteString("Lux Consensus Parameters - Detailed Report\n")
	content.WriteString("==========================================\n\n")

	content.WriteString("Network Characteristics:\n")
	content.WriteString(fmt.Sprintf("• Total Nodes: %d\n", nc.TotalNodes))
	content.WriteString(fmt.Sprintf("• Expected Byzantine Ratio: %.1f%%\n", nc.ExpectedFailureRate*100))
	content.WriteString(fmt.Sprintf("• Network Latency: %dms\n", nc.NetworkLatencyMs))
	content.WriteString(fmt.Sprintf("• Target Finality: %dms\n", nc.TargetFinalityMs))
	content.WriteString(fmt.Sprintf("• Target Throughput: %d TPS\n", nc.TargetThroughputTPS))
	content.WriteString(fmt.Sprintf("• Production: %v\n", nc.IsProduction))

	content.WriteString("\n" + Summary(p) + "\n")

	content.WriteString("\nSafety Analysis:\n")
	content.WriteString(fmt.Sprintf("• Level: %s\n", report.Level))
	if len(report.Issues) > 0 {
		content.WriteString("• Issues: " + strings.Join(report.Issues, "; ") + "\n")
	}
	if len(report.Warnings) > 0 {
		content.WriteString("• Warnings: " + strings.Join(report.Warnings, "; ") + "\n")
	}

	content.WriteString("\nProbability Analysis:\n")
	content.WriteString(fmt.Sprintf("• Safety Failure: %.2e\n", probs.SafetyFailureProbability))
	content.WriteString(fmt.Sprintf("• Liveness Failure: %.2e\n", probs.LivenessFailureProbability))
	content.WriteString(fmt.Sprintf("• Expected Rounds: %.1f\n", probs.ExpectedRoundsToFinality))

	content.WriteString("\nParameter Guide:\n")
	guides := GetParameterGuides()
	for _, guide := range guides {
		if guide.Parameter == "K (Sample Size)" ||
			guide.Parameter == "AlphaPreference" ||
			guide.Parameter == "AlphaConfidence" ||
			guide.Parameter == "Beta" {
			content.WriteString(fmt.Sprintf("\n%s:\n", guide.Parameter))
			content.WriteString(fmt.Sprintf("• %s\n", guide.Description))
			content.WriteString(fmt.Sprintf("• Impact: %s\n", guide.Impact))
			content.WriteString(fmt.Sprintf("• Trade-offs: %s\n", guide.TradeOffs))
		}
	}

	return os.WriteFile(filename, []byte(content.String()), 0644)
}

// Helper functions for prompting
func promptString(scanner *bufio.Scanner, prompt, defaultVal string) string {
	fmt.Printf("%s [%s]: ", prompt, defaultVal)
	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return defaultVal
	}
	return input
}

func promptInt(scanner *bufio.Scanner, prompt string, min, max, defaultVal int) int {
	for {
		fmt.Printf("%s [%d]: ", prompt, defaultVal)
		scanner.Scan()
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			return defaultVal
		}

		val, err := strconv.Atoi(input)
		if err != nil {
			fmt.Printf("Invalid number. Please enter a value between %d and %d.\n", min, max)
			continue
		}

		if val < min || val > max {
			fmt.Printf("Value out of range. Please enter a value between %d and %d.\n", min, max)
			continue
		}

		return val
	}
}

func promptIntDefault(scanner *bufio.Scanner, prompt string, min, max, defaultVal int) int {
	for {
		fmt.Printf("%s: ", prompt)
		scanner.Scan()
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			return defaultVal
		}

		val, err := strconv.Atoi(input)
		if err != nil {
			fmt.Printf("Invalid number. Please enter a value between %d and %d.\n", min, max)
			continue
		}

		if val < min || val > max {
			fmt.Printf("Value out of range. Please enter a value between %d and %d.\n", min, max)
			continue
		}

		return val
	}
}

func promptFloat(scanner *bufio.Scanner, prompt string, min, max, defaultVal float64) float64 {
	for {
		fmt.Printf("%s [%.2f]: ", prompt, defaultVal)
		scanner.Scan()
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			return defaultVal
		}

		val, err := strconv.ParseFloat(input, 64)
		if err != nil {
			fmt.Printf("Invalid number. Please enter a value between %.2f and %.2f.\n", min, max)
			continue
		}

		if val < min || val > max {
			fmt.Printf("Value out of range. Please enter a value between %.2f and %.2f.\n", min, max)
			continue
		}

		return val
	}
}

func promptBool(scanner *bufio.Scanner, prompt string, defaultVal bool) bool {
	defaultStr := "n"
	if defaultVal {
		defaultStr = "y"
	}

	fmt.Printf("%s [%s]: ", prompt, defaultStr)
	scanner.Scan()
	input := strings.ToLower(strings.TrimSpace(scanner.Text()))

	if input == "" {
		return defaultVal
	}

	return input == "y" || input == "yes" || input == "true" || input == "1"
}
