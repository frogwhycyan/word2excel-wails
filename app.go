package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/xuri/excelize/v2"
)

type App struct {
	ctx context.Context
}

type ConversionResult struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	TableCount int    `json:"tableCount"`
	ExcelPath  string `json:"excelPath"`
}

type FileDialogResult struct {
	Success  bool   `json:"success"`
	FilePath string `json:"filePath"`
	Message  string `json:"message"`
}

type TableData struct {
	Rows []RowData
}

type RowData struct {
	Cells []CellData
}

type CellData struct {
	Content string
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) OpenWordFile() FileDialogResult {
	filePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 Word 文档",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Word 文档 (*.docx)",
				Pattern:     "*.docx",
			},
		},
		ShowHiddenFiles: false,
	})

	if err != nil {
		return FileDialogResult{
			Success: false,
			Message: fmt.Sprintf("打开文件对话框失败: %v", err),
		}
	}

	if filePath == "" {
		return FileDialogResult{
			Success: false,
			Message: "未选择文件",
		}
	}

	return FileDialogResult{
		Success:  true,
		FilePath: filePath,
		Message:  "文件选择成功",
	}
}

func extractTablesFromDocx(filePath string) ([]TableData, error) {
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("无法打开文件: %v", err)
	}
	defer reader.Close()

	var documentXML []byte
	for _, file := range reader.File {
		if file.Name == "word/document.xml" {
			documentXML, err = readFileFromZip(file)
			if err != nil {
				return nil, fmt.Errorf("无法读取文档内容: %v", err)
			}
			break
		}
	}

	if len(documentXML) == 0 {
		return nil, fmt.Errorf("文档中未找到 word/document.xml")
	}

	return parseDocumentXML(documentXML)
}

func readFileFromZip(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, rc)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func parseDocumentXML(xmlData []byte) ([]TableData, error) {
	xmlData = bytes.TrimPrefix(xmlData, []byte("\xef\xbb\xbf"))

	type T struct {
		Text string `xml:",innerxml"`
	}

	type R struct {
		T []T `xml:"t"`
	}

	type P struct {
		R []R `xml:"r"`
	}

	type TC struct {
		P []P `xml:"p"`
	}

	type TR struct {
		TC []TC `xml:"tc"`
	}

	type Tbl struct {
		TR []TR `xml:"tr"`
	}

	type Body struct {
		Tbl []Tbl `xml:"tbl"`
	}

	type Document struct {
		Body Body `xml:"body"`
	}

	var doc Document
	if err := xml.Unmarshal(xmlData, &doc); err != nil {
		return nil, fmt.Errorf("解析 XML 失败: %v", err)
	}

	if len(doc.Body.Tbl) == 0 {
		return tryParseWithNamespace(xmlData), nil
	}

	var tables []TableData
	for _, rawTable := range doc.Body.Tbl {
		table := TableData{}
		for _, rawRow := range rawTable.TR {
			row := RowData{}
			for _, rawCell := range rawRow.TC {
				var content strings.Builder
				for _, p := range rawCell.P {
					for _, r := range p.R {
						for _, t := range r.T {
							content.WriteString(t.Text)
						}
					}
				}
				cellText := extractTextFromXML(content.String())
				row.Cells = append(row.Cells, CellData{Content: strings.TrimSpace(cellText)})
			}
			if len(row.Cells) > 0 {
				table.Rows = append(table.Rows, row)
			}
		}
		if len(table.Rows) > 0 {
			tables = append(tables, table)
		}
	}

	return tables, nil
}

func extractTextFromXML(rawXML string) string {
	var result strings.Builder
	inTag := false
	for _, r := range rawXML {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func tryParseWithNamespace(xmlData []byte) []TableData {
	type T struct {
		Text string `xml:",innerxml"`
	}

	type R struct {
		T []T `xml:"w:t"`
	}

	type P struct {
		R []R `xml:"w:r"`
	}

	type TC struct {
		P []P `xml:"w:p"`
	}

	type TR struct {
		TC []TC `xml:"w:tc"`
	}

	type Tbl struct {
		TR []TR `xml:"w:tr"`
	}

	type Body struct {
		Tbl []Tbl `xml:"w:tbl"`
	}

	type Document struct {
		Body Body `xml:"w:body"`
	}

	var doc Document
	if err := xml.Unmarshal(xmlData, &doc); err != nil {
		return nil
	}

	var tables []TableData
	for _, rawTable := range doc.Body.Tbl {
		table := TableData{}
		for _, rawRow := range rawTable.TR {
			row := RowData{}
			for _, rawCell := range rawRow.TC {
				var content strings.Builder
				for _, p := range rawCell.P {
					for _, r := range p.R {
						for _, t := range r.T {
							content.WriteString(t.Text)
						}
					}
				}
				cellText := extractTextFromXML(content.String())
				row.Cells = append(row.Cells, CellData{Content: strings.TrimSpace(cellText)})
			}
			if len(row.Cells) > 0 {
				table.Rows = append(table.Rows, row)
			}
		}
		if len(table.Rows) > 0 {
			tables = append(tables, table)
		}
	}

	return tables
}

func (a *App) ConvertWordToExcel(wordFilePath string) ConversionResult {
	if wordFilePath == "" {
		return ConversionResult{
			Success: false,
			Message: "Word 文件路径为空",
		}
	}

	fileInfo, err := os.Stat(wordFilePath)
	if err != nil {
		return ConversionResult{
			Success: false,
			Message: fmt.Sprintf("文件不存在或无法访问: %v", err),
		}
	}

	if fileInfo.IsDir() {
		return ConversionResult{
			Success: false,
			Message: "选择的路径是文件夹而非文件",
		}
	}

	if !strings.HasSuffix(strings.ToLower(wordFilePath), ".docx") {
		return ConversionResult{
			Success: false,
			Message: "请选择 .docx 格式的 Word 文档",
		}
	}

	tables, err := extractTablesFromDocx(wordFilePath)
	if err != nil {
		return ConversionResult{
			Success: false,
			Message: fmt.Sprintf("无法读取 Word 文件: %v", err),
		}
	}

	if len(tables) == 0 {
		return ConversionResult{
			Success:    false,
			Message:    "Word 文档中未找到任何表格",
			TableCount: 0,
		}
	}

	excelFile := excelize.NewFile()
	defer excelFile.Close()

	for sheetIndex, table := range tables {
		sheetName := fmt.Sprintf("Table %d", sheetIndex+1)

		if sheetIndex == 0 {
			excelFile.SetSheetName("Sheet1", sheetName)
		} else {
			_, err := excelFile.NewSheet(sheetName)
			if err != nil {
				return ConversionResult{
					Success: false,
					Message: fmt.Sprintf("创建工作表失败: %v", err),
				}
			}
		}

		for rowIdx, row := range table.Rows {
			for cellIdx, cell := range row.Cells {
				cellRef, err := excelize.CoordinatesToCellName(cellIdx+1, rowIdx+1)
				if err != nil {
					return ConversionResult{
						Success: false,
						Message: fmt.Sprintf("转换单元格坐标失败: %v", err),
					}
				}

				cellValue := strings.TrimSpace(cell.Content)
				if err := excelFile.SetCellValue(sheetName, cellRef, cellValue); err != nil {
					return ConversionResult{
						Success: false,
						Message: fmt.Sprintf("写入单元格失败: %v", err),
					}
				}
			}
		}
	}

	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title: "保存 Excel 文件",
		DefaultFilename: strings.TrimSuffix(
			strings.TrimSuffix(wordFilePath, ".docx"),
			".DOCX",
		) + ".xlsx",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Excel 文件 (*.xlsx)",
				Pattern:     "*.xlsx",
			},
		},
	})

	if err != nil {
		return ConversionResult{
			Success:    false,
			Message:    fmt.Sprintf("打开保存对话框失败: %v", err),
			TableCount: len(tables),
		}
	}

	if savePath == "" {
		return ConversionResult{
			Success:    false,
			Message:    "未选择保存路径",
			TableCount: len(tables),
		}
	}

	if !strings.HasSuffix(strings.ToLower(savePath), ".xlsx") {
		savePath += ".xlsx"
	}

	if err := excelFile.SaveAs(savePath); err != nil {
		return ConversionResult{
			Success:    false,
			Message:    fmt.Sprintf("保存 Excel 文件失败: %v", err),
			TableCount: len(tables),
		}
	}

	return ConversionResult{
		Success:    true,
		Message:    fmt.Sprintf("成功将 %d 个表格导出到 Excel 文件", len(tables)),
		TableCount: len(tables),
		ExcelPath:  savePath,
	}
}

func (a *App) ProcessDroppedFile(filePath string) ConversionResult {
	if filePath == "" {
		return ConversionResult{
			Success: false,
			Message: "拖拽的文件路径为空",
		}
	}

	if !strings.HasSuffix(strings.ToLower(filePath), ".docx") {
		return ConversionResult{
			Success: false,
			Message: "仅支持 .docx 格式的 Word 文档",
		}
	}

	return a.ConvertWordToExcel(filePath)
}
