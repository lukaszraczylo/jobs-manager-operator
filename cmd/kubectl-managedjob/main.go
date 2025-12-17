package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"raczylo.com/jobs-manager-operator/pkg/visualization"
)

var (
	namespace  string
	watch      bool
	interval   time.Duration
	noColor    bool
	kubeconfig string
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "kubectl-managedjob",
	Short: "Visualize and manage ManagedJob workflows",
	Long: `A kubectl plugin for visualizing ManagedJob workflows.

This plugin helps you understand the structure and execution status
of your ManagedJob workflows with ASCII tree visualization.`,
}

var visualizeCmd = &cobra.Command{
	Use:   "visualize <name>",
	Short: "Visualize a ManagedJob workflow tree",
	Long: `Display a ManagedJob workflow as an ASCII tree with status colors.

Status colors:
  - Green:   succeeded
  - Yellow:  running
  - Red:     failed
  - Gray:    pending
  - Magenta: aborted`,
	Args: cobra.ExactArgs(1),
	RunE: runVisualize,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all ManagedJobs in a namespace",
	RunE:  runList,
}

var statusCmd = &cobra.Command{
	Use:   "status <name>",
	Short: "Show status summary of a ManagedJob",
	Args:  cobra.ExactArgs(1),
	RunE:  runStatus,
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "default", "Kubernetes namespace")
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable colored output")

	// Visualize flags
	visualizeCmd.Flags().BoolVarP(&watch, "watch", "w", false, "Watch for changes and refresh")
	visualizeCmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "Watch refresh interval")

	// Add commands
	rootCmd.AddCommand(visualizeCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(statusCmd)
}

func runVisualize(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Handle no-color flag
	if noColor {
		color.NoColor = true
	}

	// Set KUBECONFIG if provided
	if kubeconfig != "" {
		_ = os.Setenv("KUBECONFIG", kubeconfig) // #nosec G104 - env var set failure is extremely rare
	}

	client, err := visualization.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	if watch {
		return watchLoop(ctx, client, name)
	}

	return visualizeOnce(ctx, client, name)
}

func visualizeOnce(ctx context.Context, client *visualization.Client, name string) error {
	mj, err := client.GetManagedJob(ctx, name, namespace)
	if err != nil {
		return err
	}

	tree := visualization.BuildTree(mj)
	renderer := visualization.NewRenderer(!noColor)
	fmt.Print(renderer.Render(tree))
	return nil
}

func watchLoop(ctx context.Context, client *visualization.Client, name string) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		// Clear screen and move cursor to top
		fmt.Print("\033[H\033[2J")

		if err := visualizeOnce(ctx, client, name); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}

		fmt.Printf("\nWatching %s/%s (Ctrl+C to exit, refreshing every %s)\n", namespace, name, interval)

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			continue
		}
	}
}

func runList(cmd *cobra.Command, args []string) error {
	// Handle no-color flag
	if noColor {
		color.NoColor = true
	}

	// Set KUBECONFIG if provided
	if kubeconfig != "" {
		_ = os.Setenv("KUBECONFIG", kubeconfig) // #nosec G104 - env var set failure is extremely rare
	}

	client, err := visualization.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	ctx := context.Background()
	mjList, err := client.ListManagedJobs(ctx, namespace)
	if err != nil {
		return err
	}

	if len(mjList.Items) == 0 {
		fmt.Printf("No ManagedJobs found in namespace %s\n", namespace)
		return nil
	}

	// Print table header
	fmt.Printf("%-30s %-12s %-8s %-8s\n", "NAME", "STATUS", "GROUPS", "JOBS")
	fmt.Printf("%-30s %-12s %-8s %-8s\n", "----", "------", "------", "----")

	renderer := visualization.NewRenderer(!noColor)
	_ = renderer // For potential future color support in list

	for i := range mjList.Items {
		summary := visualization.GetStatusSummary(&mjList.Items[i])
		statusStr := formatStatus(summary.Status, !noColor)
		fmt.Printf("%-30s %-12s %-8d %-8d\n", summary.Name, statusStr, summary.Groups, summary.Jobs)
	}

	return nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Handle no-color flag
	if noColor {
		color.NoColor = true
	}

	// Set KUBECONFIG if provided
	if kubeconfig != "" {
		_ = os.Setenv("KUBECONFIG", kubeconfig) // #nosec G104 - env var set failure is extremely rare
	}

	client, err := visualization.NewClient()
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	ctx := context.Background()
	mj, err := client.GetManagedJob(ctx, name, namespace)
	if err != nil {
		return err
	}

	summary := visualization.GetStatusSummary(mj)

	fmt.Printf("Name:      %s\n", summary.Name)
	fmt.Printf("Namespace: %s\n", summary.Namespace)
	fmt.Printf("Status:    %s\n", formatStatus(summary.Status, !noColor))
	fmt.Printf("Groups:    %d\n", summary.Groups)
	fmt.Printf("Jobs:      %d\n", summary.Jobs)
	fmt.Println()
	fmt.Printf("Job Status:\n")
	fmt.Printf("  Pending:   %d\n", summary.Pending)
	fmt.Printf("  Running:   %d\n", summary.Running)
	fmt.Printf("  Succeeded: %d\n", summary.Succeeded)
	fmt.Printf("  Failed:    %d\n", summary.Failed)
	fmt.Printf("  Aborted:   %d\n", summary.Aborted)

	return nil
}

func formatStatus(status string, useColor bool) string {
	if !useColor {
		return status
	}

	switch status {
	case visualization.StatusSucceeded:
		return color.GreenString(status)
	case visualization.StatusRunning:
		return color.YellowString(status)
	case visualization.StatusFailed:
		return color.RedString(status)
	case visualization.StatusPending:
		return color.HiBlackString(status)
	case visualization.StatusAborted:
		return color.MagentaString(status)
	default:
		return status
	}
}
