package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	API_BASE = "http://localhost:8080"
)

type TestResult struct {
	TestName string
	Status   string
	Message  string
	Duration time.Duration
}

var results []TestResult

func main() {
	fmt.Println("=====================================")
	fmt.Println("   FILE-SERVER FILEBOX QA TESTING")
	fmt.Println("=====================================")
	fmt.Println("")

	// Test 1: Health Check
	testHealthCheck()

	// Test 2: Valid Filebox Name - Initiate Upload
	testInitiateUpload("myfilebox", 5242880, true) // 5MB

	// Test 3: Valid Filebox Name with Numbers
	testInitiateUpload("filebox123", 1048576, true) // 1MB

	// Test 4: Uppercase Filebox Name
	testInitiateUpload("MYFILES", 2097152, true) // 2MB

	// Test 5: Mixed Case Filebox Name
	testInitiateUpload("MyFileBox456", 3145728, true) // 3MB

	// Test 6: Invalid Filebox Name - Empty
	testInitiateUpload("", 1048576, false)

	// Test 7: Invalid Filebox Name - Special Characters
	testInitiateUpload("my-filebox", 1048576, false)

	// Test 8: Invalid Filebox Name - Space
	testInitiateUpload("my filebox", 1048576, false)

	// Test 9: Invalid Filebox Name - Underscore
	testInitiateUpload("my_filebox", 1048576, false)

	// Test 10: Invalid Filebox Name - Symbols
	testInitiateUpload("my@filebox", 1048576, false)

	// Test 11: Single Character Filebox Name
	testInitiateUpload("a", 1048576, true)

	// Test 12: Long Alphanumeric Filebox Name
	testInitiateUpload("thisisaverylongfileboxnamewithlotsofcharacters1234567890", 1048576, true)

	// Test 13: List Uploads - Valid Filebox
	testListUploads("myfilebox", true)

	// Test 14: List Uploads - Invalid Filebox (with special chars)
	testListUploads("my-filebox", false)

	// Test 15: List Uploads - Empty Filebox Name
	testListUploads("", false)

	// Test 16: List Uploads - Non-existent Filebox
	testListUploads("nonexistentfilebox", true) // Should return empty array, not error

	// Test 17: Initiate with Zero Size
	testInitiateUpload("testfilebox", 0, false)

	// Test 18: Initiate with Negative Size
	testInitiateUpload("testfilebox", -1048576, false)

	// Test 19: Valid filebox - Numeric Only
	testInitiateUpload("123456", 1048576, true)

	// Test 20: Valid filebox - Alphanumeric Mix
	testInitiateUpload("abc123xyz789", 1048576, true)

	// Print Summary
	printSummary()
}

func testHealthCheck() {
	start := time.Now()
	testName := "Health Check"

	resp, err := http.Get(API_BASE + "/health")
	if err != nil {
		recordResult(testName, "FAIL", fmt.Sprintf("Request failed: %v", err), time.Since(start))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 200 {
		recordResult(testName, "PASS", string(body), time.Since(start))
	} else {
		recordResult(testName, "FAIL", fmt.Sprintf("Expected 200, got %d: %s", resp.StatusCode, string(body)), time.Since(start))
	}
}

func testInitiateUpload(fileboxName string, size int64, shouldPass bool) {
	start := time.Now()
	testName := fmt.Sprintf("Initiate Upload - Filebox: '%s', Size: %d bytes", fileboxName, size)

	payload := map[string]interface{}{
		"key":         "test-document.pdf",
		"size":        size,
		"fileboxName": fileboxName,
	}

	jsonData, _ := json.Marshal(payload)
	resp, err := http.Post(API_BASE+"/initiate-multipart", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		recordResult(testName, "FAIL", fmt.Sprintf("Request failed: %v", err), time.Since(start))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if shouldPass {
		if resp.StatusCode == 200 {
			var response map[string]interface{}
			json.Unmarshal(body, &response)
			uploadID := response["uploadId"]
			recordResult(testName, "PASS", fmt.Sprintf("Upload initiated with ID: %v", uploadID), time.Since(start))
		} else {
			recordResult(testName, "FAIL", fmt.Sprintf("Expected 200, got %d: %s", resp.StatusCode, string(body)), time.Since(start))
		}
	} else {
		if resp.StatusCode != 200 {
			recordResult(testName, "PASS", fmt.Sprintf("Correctly rejected with %d: %s", resp.StatusCode, string(body)), time.Since(start))
		} else {
			recordResult(testName, "FAIL", fmt.Sprintf("Should have been rejected but got 200: %s", string(body)), time.Since(start))
		}
	}
}

func testListUploads(fileboxName string, shouldPass bool) {
	start := time.Now()
	testName := fmt.Sprintf("List Uploads - Filebox: '%s'", fileboxName)

	url := API_BASE + "/list-uploads"
	if fileboxName != "" {
		url += "?filebox=" + fileboxName
	}

	resp, err := http.Get(url)
	if err != nil {
		recordResult(testName, "FAIL", fmt.Sprintf("Request failed: %v", err), time.Since(start))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if shouldPass {
		if resp.StatusCode == 200 {
			var response map[string]interface{}
			json.Unmarshal(body, &response)
			uploads := response["uploads"]
			recordResult(testName, "PASS", fmt.Sprintf("Listed uploads: %v", uploads), time.Since(start))
		} else {
			recordResult(testName, "FAIL", fmt.Sprintf("Expected 200, got %d: %s", resp.StatusCode, string(body)), time.Since(start))
		}
	} else {
		if resp.StatusCode != 200 {
			recordResult(testName, "PASS", fmt.Sprintf("Correctly rejected with %d: %s", resp.StatusCode, string(body)), time.Since(start))
		} else {
			recordResult(testName, "FAIL", fmt.Sprintf("Should have been rejected but got 200: %s", string(body)), time.Since(start))
		}
	}
}

func recordResult(testName, status, message string, duration time.Duration) {
	result := TestResult{
		TestName: testName,
		Status:   status,
		Message:  message,
		Duration: duration,
	}
	results = append(results, result)

	// Print in real-time
	statusSymbol := "✓"
	if status == "FAIL" {
		statusSymbol = "✗"
	}
	fmt.Printf("[%s] %s\n", statusSymbol, testName)
	fmt.Printf("    Status: %s | Duration: %v\n", status, duration)
	fmt.Printf("    Details: %s\n\n", message)
}

func printSummary() {
	fmt.Println("")
	fmt.Println("=====================================")
	fmt.Println("              TEST SUMMARY")
	fmt.Println("=====================================")

	passCount := 0
	failCount := 0
	totalDuration := time.Duration(0)

	for _, result := range results {
		if result.Status == "PASS" {
			passCount++
		} else {
			failCount++
		}
		totalDuration += result.Duration
	}

	fmt.Printf("Total Tests:    %d\n", len(results))
	fmt.Printf("Passed:         %d\n", passCount)
	fmt.Printf("Failed:         %d\n", failCount)
	fmt.Printf("Pass Rate:      %.1f%%\n", float64(passCount)/float64(len(results))*100)
	fmt.Printf("Total Duration: %v\n", totalDuration)
	fmt.Println("")

	if failCount > 0 {
		fmt.Println("Failed Tests:")
		for _, result := range results {
			if result.Status == "FAIL" {
				fmt.Printf("  - %s\n", result.TestName)
				fmt.Printf("    %s\n", result.Message)
			}
		}
		fmt.Println("")
	}

	if failCount == 0 {
		fmt.Println("🎉 All tests passed!")
	} else {
		fmt.Printf("⚠️  %d test(s) failed\n", failCount)
	}
	fmt.Println("=====================================")
}
