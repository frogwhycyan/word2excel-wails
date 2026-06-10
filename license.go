package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	PocketBaseURL = "http://212.64.16.254:8090"
	LicenseFile   = "word2excel.license"
)

// LicenseInfo 保存在本地的激活信息
type LicenseInfo struct {
	MachineID      string `json:"machineId"`
	ActivationCode string `json:"activationCode"`
}

// LicenseCheckResult 激活验证结果
type LicenseCheckResult struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message"`
}

// ActivationResult 激活操作结果
type ActivationResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// getLicenseFilePath 获取许可证文件路径（用户目录下）
func getLicenseFilePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}
	return filepath.Join(homeDir, LicenseFile)
}

// generateMachineID 生成机器码（基于操作系统信息）
func generateMachineID() string {
	hostname, _ := os.Hostname()
	username := ""
	switch runtime.GOOS {
	case "windows":
		username = os.Getenv("USERNAME")
	case "darwin", "linux":
		username = os.Getenv("USER")
	}

	data := fmt.Sprintf("%s-%s-%s", hostname, username, runtime.GOOS)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])[:15]
}

// GetMachineID 获取本机机器码（暴露给前端）
func (a *App) GetMachineID() string {
	return generateMachineID()
}

// readLocalLicense 读取本地保存的许可证信息
func readLocalLicense() (*LicenseInfo, error) {
	filePath := getLicenseFilePath()
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var license LicenseInfo
	if err := json.Unmarshal(data, &license); err != nil {
		return nil, err
	}

	return &license, nil
}

// saveLocalLicense 保存许可证信息到本地
func saveLocalLicense(license *LicenseInfo) error {
	filePath := getLicenseFilePath()
	data, err := json.Marshal(license)
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

// CheckLicense 检查软件是否已激活（暴露给前端）
func (a *App) CheckLicense() LicenseCheckResult {
	// 1. 读取本地许可证
	license, err := readLocalLicense()
	if err != nil {
		return LicenseCheckResult{
			Valid:   false,
			Message: "未找到激活信息，请先激活软件",
		}
	}

	// 2. 验证机器码是否匹配
	machineID := generateMachineID()
	if license.MachineID != machineID {
		return LicenseCheckResult{
			Valid:   false,
			Message: "机器码不匹配，请重新激活",
		}
	}

	// 3. 向 PocketBase 验证激活状态
	return verifyLicenseWithServer(machineID, license.ActivationCode)
}

// verifyLicenseWithServer 向 PocketBase 服务器验证激活码
func verifyLicenseWithServer(machineID, activationCode string) LicenseCheckResult {
	// 使用视图 word2excel_licenseView 查询
	// 构造查询参数
	params := url.Values{}
	params.Add("filter", fmt.Sprintf("machineid='%s' && activationcode='%s' && status=true", machineID, activationCode))
	params.Add("perPage", "1")

	// 拼接完整的 URL
	url := fmt.Sprintf("%s/api/collections/word2excel_licenseView/records?%s", PocketBaseURL, params.Encode())

	// // 别忘了在文件头部 import "net/url"
	// filter := fmt.Sprintf("machineid='%s' && activationcode='%s' && status=true", machineID, activationCode)
	// url := fmt.Sprintf("%s/api/collections/word2excel_licenseView/records?filter=%s&perPage=1", PocketBaseURL, filter)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	println(url)
	println(resp.StatusCode)
	println(resp.Status)
	println(resp)
	if err != nil {
		return LicenseCheckResult{
			Valid:   false,
			Message: "无法连接到授权服务器，请检查网络",
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return LicenseCheckResult{
			Valid:   false,
			Message: "授权服务器响应异常",
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return LicenseCheckResult{
			Valid:   false,
			Message: "读取授权服务器响应失败",
		}
	}

	var result struct {
		Items []struct {
			ID             string `json:"id"`
			MachineID      string `json:"machineid"`
			ActivationCode string `json:"activationcode"`
			ExpiryDate     string `json:"expiry_date"`
			Status         bool   `json:"status"`
		} `json:"items"`
		TotalItems int `json:"totalItems"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return LicenseCheckResult{
			Valid:   false,
			Message: "解析授权服务器响应失败",
		}
	}

	if result.TotalItems == 0 || len(result.Items) == 0 {
		return LicenseCheckResult{
			Valid:   false,
			Message: "激活码无效或已被禁用",
		}
	}

	record := result.Items[0]

	// 检查是否过期
	if record.ExpiryDate != "" {
		// PocketBase 的日期格式通常是 "2006-01-02 15:04:05.000Z"
		// Go 中最安全的解析方式是尝试将其当做标准时间解析
		// 如果 PocketBase 返回的是标准格式，可以用 time.ParseDataSource 或以下常规格式：

		// 兼容 PocketBase 常见的两种时间格式
		var expiryDate time.Time
		var err error

		// 尝试带 T 的标准 RFC3339 格式
		expiryDate, err = time.Parse(time.RFC3339, record.ExpiryDate)
		if err != nil {
			// 尝试 PocketBase 默认的 "2006-01-02 15:04:05.000Z" 格式
			expiryDate, err = time.Parse("2006-01-02 15:04:05.000Z", record.ExpiryDate)
		}

		if err == nil {
			// 统一转换为本地时间，并只保留到“天”进行比较
			localExpiry := expiryDate.Local().Truncate(24 * time.Hour)
			today := time.Now().Local().Truncate(24 * time.Hour)

			// 如果今天已经严格在过期日期之后，则代表过期
			if today.After(localExpiry) {
				return LicenseCheckResult{
					Valid:   false,
					Message: "激活码已过期，请联系管理员续期",
				}
			}
		} else {
			// 调试用：如果解析失败，可以在控制台打印出来看看 PocketBase 到底返回了什么格式
			println("时间解析失败，实际收到的时间字符串为:", record.ExpiryDate, "错误原因:", err.Error())
		}
	}

	return LicenseCheckResult{
		Valid:   true,
		Message: "软件已激活",
	}
}

// Activate 使用激活码激活软件（暴露给前端）
func (a *App) Activate(activationCode string) ActivationResult {
	machineID := generateMachineID()

	// 1. 检查激活码格式
	activationCode = strings.TrimSpace(activationCode)
	if activationCode == "" {
		return ActivationResult{
			Success: false,
			Message: "激活码不能为空",
		}
	}

	// 2. 查询 licenses 表，检查激活码是否存在且未使用
	licenseRecord, err := findLicenseRecord(activationCode)
	if err != nil {
		return ActivationResult{
			Success: false,
			Message: err.Error(),
		}
	}

	// 3. 创建 machines 记录
	machineRecordID, err := createMachineRecord(machineID)
	if err != nil {
		return ActivationResult{
			Success: false,
			Message: fmt.Sprintf("创建机器记录失败: %v", err),
		}
	}

	// 4. 更新 licenses 记录：设置 status=true，关联 machinecode
	if err := updateLicenseRecord(licenseRecord.ID, machineRecordID); err != nil {
		return ActivationResult{
			Success: false,
			Message: fmt.Sprintf("更新许可证记录失败: %v", err),
		}
	}

	// 5. 保存本地许可证
	license := &LicenseInfo{
		MachineID:      machineID,
		ActivationCode: activationCode,
	}
	if err := saveLocalLicense(license); err != nil {
		return ActivationResult{
			Success: false,
			Message: fmt.Sprintf("保存本地激活信息失败: %v", err),
		}
	}

	return ActivationResult{
		Success: true,
		Message: "激活成功！",
	}
}

// LicenseRecord 许可证记录
type LicenseRecord struct {
	ID             string `json:"id"`
	ActivationCode string `json:"activationcode"`
	Status         bool   `json:"status"`
	MachineCode    string `json:"machinecode"`
}

// findLicenseRecord 根据激活码查找许可证记录
func findLicenseRecord(activationCode string) (*LicenseRecord, error) {
	filter := fmt.Sprintf("activationcode='%s'", activationCode)
	url := fmt.Sprintf("%s/api/collections/licenses/records?filter=%s&perPage=1", PocketBaseURL, filter)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("无法连接到授权服务器")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("授权服务器响应异常")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败")
	}

	var result struct {
		Items      []LicenseRecord `json:"items"`
		TotalItems int             `json:"totalItems"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析响应失败")
	}

	if result.TotalItems == 0 || len(result.Items) == 0 {
		return nil, fmt.Errorf("激活码不存在")
	}

	record := result.Items[0]

	// 检查是否已被使用
	if record.Status {
		return nil, fmt.Errorf("激活码已被使用")
	}

	return &record, nil
}

// createMachineRecord 创建机器记录
func createMachineRecord(machineID string) (string, error) {
	url := fmt.Sprintf("%s/api/collections/machines/records", PocketBaseURL)

	data := map[string]string{
		"machineid": machineID,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("服务器返回 %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	return result.ID, nil
}

// updateLicenseRecord 更新许可证记录
func updateLicenseRecord(licenseID, machineCodeID string) error {
	url := fmt.Sprintf("%s/api/collections/licenses/records/%s", PocketBaseURL, licenseID)

	data := map[string]interface{}{
		"status":      true,
		"machinecode": machineCodeID,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("服务器返回 %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
