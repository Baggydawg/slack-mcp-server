// Command compress-test is a CLI tool for testing PNG to JPEG compression.
// It accepts PNG files or directories containing PNG files and outputs
// compressed JPEG files, reporting the compression ratio achieved.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultQuality   = 80
	defaultOutputDir = "test/compression/output"
)

func main() {
	inputPath := flag.String("input", "", "Input PNG file or directory containing PNG files")
	quality := flag.Int("quality", defaultQuality, "JPEG quality (1-100)")
	outputDir := flag.String("output", defaultOutputDir, "Output directory for compressed JPEGs")
	flag.Parse()

	if *inputPath == "" {
		fmt.Fprintln(os.Stderr, "Error: -input flag is required")
		flag.Usage()
		os.Exit(1)
	}

	if *quality < 1 || *quality > 100 {
		fmt.Fprintln(os.Stderr, "Error: quality must be between 1 and 100")
		os.Exit(1)
	}

	// Create output directory if needed
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	// Check if input is a file or directory
	info, err := os.Stat(*inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error accessing input path: %v\n", err)
		os.Exit(1)
	}

	var files []string
	if info.IsDir() {
		// Find all PNG files in directory
		entries, err := os.ReadDir(*inputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading directory: %v\n", err)
			os.Exit(1)
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.ToLower(filepath.Ext(entry.Name())) == ".png" {
				files = append(files, filepath.Join(*inputPath, entry.Name()))
			}
		}
		if len(files) == 0 {
			fmt.Fprintln(os.Stderr, "No PNG files found in input directory")
			os.Exit(1)
		}
	} else {
		if strings.ToLower(filepath.Ext(*inputPath)) != ".png" {
			fmt.Fprintln(os.Stderr, "Error: input file must be a PNG")
			os.Exit(1)
		}
		files = []string{*inputPath}
	}

	// Process each file
	var totalOriginal, totalCompressed int64
	for _, inputFile := range files {
		originalSize, compressedSize, err := compressFile(inputFile, *outputDir, *quality)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", filepath.Base(inputFile), err)
			continue
		}

		reduction := 100.0 - (float64(compressedSize) / float64(originalSize) * 100.0)
		fmt.Printf("%s: %s -> %s (%.0f%% reduction) @ quality %d\n",
			filepath.Base(inputFile),
			formatBytes(originalSize),
			formatBytes(compressedSize),
			reduction,
			*quality,
		)

		totalOriginal += originalSize
		totalCompressed += compressedSize
	}

	// Print summary if multiple files
	if len(files) > 1 {
		totalReduction := 100.0 - (float64(totalCompressed) / float64(totalOriginal) * 100.0)
		fmt.Printf("\nTotal: %s -> %s (%.0f%% reduction)\n",
			formatBytes(totalOriginal),
			formatBytes(totalCompressed),
			totalReduction,
		)
	}
}

// compressFile reads a PNG file, compresses it to JPEG, and returns the original and compressed sizes.
func compressFile(inputPath, outputDir string, quality int) (int64, int64, error) {
	// Get original file size
	inputInfo, err := os.Stat(inputPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to stat input file: %w", err)
	}
	originalSize := inputInfo.Size()

	// Open and decode PNG
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open input file: %w", err)
	}
	defer inputFile.Close()

	img, err := png.Decode(inputFile)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to decode PNG: %w", err)
	}

	// Create output file
	baseName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	outputPath := filepath.Join(outputDir, baseName+".jpg")

	outputFile, err := os.Create(outputPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	// Encode as JPEG
	opts := &jpeg.Options{Quality: quality}
	if err := jpeg.Encode(outputFile, img, opts); err != nil {
		return 0, 0, fmt.Errorf("failed to encode JPEG: %w", err)
	}

	// Get compressed file size
	outputInfo, err := os.Stat(outputPath)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to stat output file: %w", err)
	}
	compressedSize := outputInfo.Size()

	return originalSize, compressedSize, nil
}

// formatBytes formats a byte count into a human-readable string (KB, MB, etc.)
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1fGB", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.0fKB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// Ensure image is imported for side effects (image format registration)
var _ image.Image
