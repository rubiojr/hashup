package store

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// ProcessStats tracks statistics for the NATS consumer
type ProcessStats struct {
	messagesReceived int64
	recordsWritten   int64
	recordsSkipped   int64
	recordsPresent   int64
	filesByExtension map[string]int64
	hostStats        map[string]int64
	lastUpdateTime   time.Time
	startTime        time.Time
	mutex            sync.Mutex
}

// NewProcessStats creates a new statistics tracker
func NewProcessStats() *ProcessStats {
	return &ProcessStats{
		filesByExtension: make(map[string]int64),
		hostStats:        make(map[string]int64),
		startTime:        time.Now(),
		lastUpdateTime:   time.Now(),
	}
}

// IncrementReceived increases the count of received messages
func (stats *ProcessStats) IncrementReceived() {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()
	stats.messagesReceived++
}

// IncrementWritten increases the count of records written to the database
func (stats *ProcessStats) IncrementWritten() {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()
	stats.recordsWritten++
}

// IncrementSkipped increases the count of records skipped
func (stats *ProcessStats) IncrementSkipped() {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()
	stats.recordsSkipped++
}

func (stats *ProcessStats) IncrementAlreadyPresent() {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()
	stats.recordsPresent++
}

// RecordExtension adds a file extension to the statistics
func (stats *ProcessStats) RecordExtension(ext string) {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()
	if ext == "" {
		ext = "[no extension]"
	}
	stats.filesByExtension[ext]++
}

// RecordHost adds a host to the statistics
func (stats *ProcessStats) RecordHost(host string) {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()
	stats.hostStats[host]++
}

// PrintLiveStatus prints a one-line status update
func (stats *ProcessStats) PrintLiveStatus() {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()
	fmt.Printf("\rProcessed: %d written, %d skipped, %d present",
		stats.recordsWritten, stats.recordsSkipped, stats.recordsPresent)
}

// PrintStats prints detailed statistics
func (stats *ProcessStats) PrintStats() {
	stats.mutex.Lock()
	defer stats.mutex.Unlock()

	now := time.Now()
	elapsed := now.Sub(stats.startTime)
	//timeSinceUpdate := now.Sub(stats.lastUpdateTime)

	// Calculate rates per second
	var msgsPerSec, writtenPerSec, skippedPerSec, presentPerSec float64
	if elapsed.Seconds() > 0 {
		msgsPerSec = float64(stats.messagesReceived) / elapsed.Seconds()
		writtenPerSec = float64(stats.recordsWritten) / elapsed.Seconds()
		skippedPerSec = float64(stats.recordsSkipped) / elapsed.Seconds()
		presentPerSec = float64(stats.recordsPresent) / elapsed.Seconds()
	}

	fmt.Println("\n-------------------------")
	fmt.Printf("HASHUP STATS (elapsed %s)\n", elapsed.Round(time.Second))
	fmt.Println("-------------------------")
	fmt.Printf("Messages received: %d (%.1f/sec)\n", stats.messagesReceived, msgsPerSec)
	fmt.Printf("Records written:   %d (%.1f/sec)\n", stats.recordsWritten, writtenPerSec)
	fmt.Printf("Records skipped:   %d (%.1f/sec)\n", stats.recordsSkipped, skippedPerSec)
	fmt.Printf("Records present:   %d (%.1f/sec)\n", stats.recordsPresent, presentPerSec)

	if len(stats.hostStats) > 0 {
		fmt.Println("\nHosts:")
		for host, count := range stats.hostStats {
			fmt.Printf("  %-20s %d\n", host, count)
		}
	}

	if len(stats.filesByExtension) > 0 {
		fmt.Println("\nTop file extensions:")
		// Create a sorted list of extensions by count
		type extCount struct {
			ext   string
			count int64
		}

		var extCounts []extCount
		for ext, count := range stats.filesByExtension {
			extCounts = append(extCounts, extCount{ext, count})
		}

		// Sort by count (descending)
		// Simple bubble sort for clarity
		for i := 0; i < len(extCounts)-1; i++ {
			for j := 0; j < len(extCounts)-i-1; j++ {
				if extCounts[j].count < extCounts[j+1].count {
					extCounts[j], extCounts[j+1] = extCounts[j+1], extCounts[j]
				}
			}
		}

		// Print top 10 or all if less than 10
		limit := 10
		if len(extCounts) < limit {
			limit = len(extCounts)
		}

		for i := 0; i < limit; i++ {
			fmt.Printf("  %-20s %d\n", extCounts[i].ext, extCounts[i].count)
		}
	}

	fmt.Println("-------------------------")

	// Update the last update time
	stats.lastUpdateTime = now
}

// StartStatsPrinters starts the background goroutines for printing statistics
func (stats *ProcessStats) StartStatsPrinters(ctx context.Context, statsInterval int) {
	// Start the periodic full stats printer if interval > 0
	if statsInterval > 0 {
		go func() {
			ticker := time.NewTicker(time.Duration(statsInterval) * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					stats.PrintStats()
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	// Start the live status printer
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				stats.PrintLiveStatus()
			case <-ctx.Done():
				return
			}
		}
	}()
}
