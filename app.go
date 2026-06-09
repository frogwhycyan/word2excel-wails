package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
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
	Content  string
	Image    []byte
	ImageExt string
	GridSpan int  // 水平合并列数
	IsVMerge bool // 是否垂直合并
	IsVStart bool // 是否是垂直合并起始
}

type Relationship struct {
	Id     string `xml:"Id,attr"`
	Type   string `xml:"Type,attr"`
	Target string `xml:"Target,attr"`
}

type Relationships struct {
	Relationships []Relationship `xml:"Relationship"`
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

	images := make(map[string][]byte)
	for _, file := range reader.File {
		if strings.HasPrefix(file.Name, "word/media/") && !file.FileInfo().IsDir() {
			imageData, err := readFileFromZip(file)
			if err == nil {
				images[file.Name] = imageData
			}
		}
	}

	idToImage := make(map[string]string)
	for _, file := range reader.File {
		if file.Name == "word/_rels/document.xml.rels" {
			relsContent, err := readFileFromZip(file)
			if err == nil {
				var rels Relationships
				if xml.Unmarshal(relsContent, &rels) == nil {
					for _, rel := range rels.Relationships {
						if strings.HasPrefix(rel.Target, "media/") {
							idToImage[rel.Id] = "word/" + rel.Target
						}
					}
				}
			}
		}
	}

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

	return parseTablesWithImages(documentXML, images, idToImage)
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

func parseTablesWithImages(xmlData []byte, images map[string][]byte, idToImage map[string]string) ([]TableData, error) {
	xmlData = bytes.TrimPrefix(xmlData, []byte("\xef\xbb\xbf"))
	content := string(xmlData)

	var tables []TableData
	currentPos := 0

	for {
		tblStart := strings.Index(content[currentPos:], "<w:tbl")
		if tblStart == -1 {
			break
		}
		tblStart += currentPos

		tagEnd := strings.Index(content[tblStart:], ">")
		if tagEnd == -1 {
			currentPos = tblStart + 7
			continue
		}
		tagEnd += tblStart + 1

		tblEnd := findEndTag(content, tblStart, "w:tbl")
		if tblEnd == -1 {
			currentPos = tagEnd
			continue
		}

		tableXML := content[tblStart:tblEnd]
		table := parseSingleTable(tableXML, images, idToImage)
		if len(table.Rows) > 0 {
			tables = append(tables, table)
		}

		currentPos = tblEnd
	}

	if len(tables) == 0 {
		tables = tryParseWithNamespace(xmlData)
	}

	return tables, nil
}

func findEndTag(content string, start int, tagName string) int {
	startTag := "<" + tagName + ">"
	endTag := "</" + tagName + ">"

	depth := 1
	searchStart := start + len(startTag)

	for depth > 0 {
		nextStart := strings.Index(content[searchStart:], startTag)
		nextEnd := strings.Index(content[searchStart:], endTag)

		if nextEnd == -1 {
			return -1
		}

		if nextStart == -1 || nextEnd < nextStart {
			depth--
			if depth == 0 {
				return searchStart + nextEnd + len(endTag)
			}
			searchStart += nextEnd + len(endTag)
		} else {
			depth++
			searchStart += nextStart + len(startTag)
		}
	}

	return -1
}

func parseSingleTable(tableXML string, images map[string][]byte, idToImage map[string]string) TableData {
	var table TableData

	rowStart := 0
	for {
		trStart := strings.Index(tableXML[rowStart:], "<w:tr")
		if trStart == -1 {
			break
		}
		trStart += rowStart

		tagEnd := strings.Index(tableXML[trStart:], ">")
		if tagEnd == -1 {
			rowStart = trStart + 6
			continue
		}

		trEnd := findEndTag(tableXML, trStart, "w:tr")
		if trEnd == -1 {
			rowStart = trStart + tagEnd + 1
			continue
		}

		row := parseRow(tableXML[trStart:trEnd], images, idToImage)
		if len(row.Cells) > 0 {
			table.Rows = append(table.Rows, row)
		}

		rowStart = trEnd
	}

	return table
}

func parseRow(rowXML string, images map[string][]byte, idToImage map[string]string) RowData {
	var row RowData
	tcStart := 0

	for {
		tcStartIdx := strings.Index(rowXML[tcStart:], "<w:tc")
		if tcStartIdx == -1 {
			break
		}
		tcStartIdx += tcStart

		tagEnd := strings.Index(rowXML[tcStartIdx:], ">")
		if tagEnd == -1 {
			tcStart = tcStartIdx + 6
			continue
		}

		tcEnd := findEndTag(rowXML, tcStartIdx, "w:tc")
		if tcEnd == -1 {
			tcStart = tcStartIdx + tagEnd + 1
			continue
		}

		cell := parseCell(rowXML[tcStartIdx:tcEnd], images, idToImage)
		row.Cells = append(row.Cells, cell)

		tcStart = tcEnd
	}

	return row
}

func extractGridSpan(cellXML string) int {
	gridSpanStart := strings.Index(cellXML, "<w:gridSpan")
	if gridSpanStart == -1 {
		return 1
	}

	valStart := strings.Index(cellXML[gridSpanStart:], "w:val=\"")
	if valStart == -1 {
		return 1
	}
	valStart += gridSpanStart + 7

	valEnd := strings.Index(cellXML[valStart:], "\"")
	if valEnd == -1 {
		return 1
	}

	var span int
	fmt.Sscanf(cellXML[valStart:valStart+valEnd], "%d", &span)
	if span < 1 {
		span = 1
	}
	return span
}

func extractVMerge(cellXML string) (bool, bool) {
	vMergeStart := strings.Index(cellXML, "<w:vMerge")
	if vMergeStart == -1 {
		return false, false
	}

	valStart := strings.Index(cellXML[vMergeStart:], "w:val=\"")
	if valStart == -1 {
		// WordprocessingML 中 <w:vMerge/> 通常表示“继续合并”（continue）
		return true, false
	}
	valStart += vMergeStart + 7

	valEnd := strings.Index(cellXML[valStart:], "\"")
	if valEnd == -1 {
		return true, false
	}

	val := cellXML[valStart : valStart+valEnd]
	if val == "restart" {
		return true, true
	}
	return true, false
}

func parseCell(cellXML string, images map[string][]byte, idToImage map[string]string) CellData {
	var cell CellData
	var textBuffer strings.Builder

	if strings.Contains(cellXML, "r:embed=\"") {
		embedIdx := strings.Index(cellXML, "r:embed=\"")
		if embedIdx != -1 {
			embedIdx += 9
			quoteIdx := strings.Index(cellXML[embedIdx:], "\"")
			if quoteIdx != -1 {
				embedId := cellXML[embedIdx : embedIdx+quoteIdx]
				if imagePath, ok := idToImage[embedId]; ok {
					if imageData, ok := images[imagePath]; ok {
						cell.Image = imageData
						cell.ImageExt = filepath.Ext(imagePath)
					}
				}
			}
		}
	}

	cell.GridSpan = extractGridSpan(cellXML)
	cell.IsVMerge, cell.IsVStart = extractVMerge(cellXML)

	tStart := 0
	for {
		tIdx := strings.Index(cellXML[tStart:], "<w:t>")
		if tIdx == -1 {
			break
		}
		tIdx += tStart + 5

		tEndIdx := strings.Index(cellXML[tIdx:], "</w:t>")
		if tEndIdx == -1 {
			break
		}

		textBuffer.WriteString(cellXML[tIdx : tIdx+tEndIdx])
		tStart = tIdx + tEndIdx + 6
	}

	cell.Content = strings.TrimSpace(textBuffer.String())
	return cell
}

func detectImageFormat(data []byte) string {
	if len(data) < 4 {
		return ""
	}
	if data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF {
		return "jpeg"
	}
	if data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47 {
		return "png"
	}
	if data[0] == 0x47 && data[1] == 0x49 && data[2] == 0x46 && data[3] == 0x38 {
		return "gif"
	}
	if len(data) >= 12 && string(data[0:6]) == "GIF87a" || string(data[0:6]) == "GIF89a" {
		return "gif"
	}
	return ""
}

func normalizeImageExtension(ext string) string {
	switch ext {
	case "jpg", "jpeg", "jpe":
		return "jpeg"
	case "png":
		return "png"
	case "gif":
		return "gif"
	case "bmp":
		return "bmp"
	case "tiff", "tif":
		return "tiff"
	default:
		return ""
	}
}

func reencodeImage(data []byte, ext string) ([]byte, string) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data, ext
	}

	var buf bytes.Buffer

	switch ext {
	case ".jpeg":
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
			return data, ext
		}
	case ".png":
		if err := png.Encode(&buf, img); err != nil {
			return data, ext
		}
	case ".gif":
		return data, ext
	default:
		if err := png.Encode(&buf, img); err != nil {
			return data, ext
		}
		ext = ".png"
	}

	return buf.Bytes(), ext
}

func imageDimensions(data []byte) (int, int) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return 0, 0
	}
	bounds := img.Bounds()
	return bounds.Dx(), bounds.Dy()
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

type mergeAction struct {
	start string
	end   string
}

type vMergeState struct {
	startRow int
	startCol int
	span     int
}

type pictureAction struct {
	cellRef string
	pic     *excelize.Picture
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
			if _, err := excelFile.NewSheet(sheetName); err != nil {
				return ConversionResult{
					Success: false,
					Message: fmt.Sprintf("创建工作表失败: %v", err),
				}
			}
		}

		// occupied 用于标记本行中被水平合并占用的列，避免后续单元格落到已占用列
		occupied := make(map[string]bool)
		var mergeActions []mergeAction
		activeVMerge := make(map[int]vMergeState) // key = 起始列（startCol）
		var pictureActions []pictureAction

		for rowIdx, row := range table.Rows {
			excelRowIdx := rowIdx + 1
			cursorCol := 1
			usedVMerge := make(map[int]bool) // 当前行已消费的 vMerge continuation（按 startCol 记录）

			for _, cell := range row.Cells {
				span := cell.GridSpan
				if span < 1 {
					span = 1
				}

				isContinuation := cell.IsVMerge && !cell.IsVStart
				if isContinuation {
					// continuation 单元格必须落在“正在进行的纵向合并”的那一列，否则会导致后续错位
					nextCol := -1
					var nextSt vMergeState
					for startCol, st := range activeVMerge {
						if usedVMerge[startCol] {
							continue
						}
						if startCol >= cursorCol && (nextCol == -1 || startCol < nextCol) {
							nextCol = startCol
							nextSt = st
						}
					}
					actualCol := cursorCol
					if nextCol != -1 {
						actualCol = nextCol
						span = nextSt.span // continuation 的 span 以起始单元格为准
						usedVMerge[nextCol] = true
					}
					rangeStart, rangeEnd := actualCol, actualCol+span-1

					for c := rangeStart; c <= rangeEnd; c++ {
						occupied[fmt.Sprintf("%d,%d", excelRowIdx, c)] = true
					}
					cursorCol = rangeEnd + 1
					continue
				}

				// 普通单元格：在落位前，若当前位置落在某个仍在进行的 vMerge 矩形内，
				// 但本行并未提供 continuation，则说明该 vMerge 在上一行已经结束，需要先闭合。
				closeOverlappedVMerge := func(col int) bool {
					for startCol, st := range activeVMerge {
						if usedVMerge[startCol] {
							continue
						}
						if col >= startCol && col <= startCol+st.span-1 {
							endRow := excelRowIdx - 1
							if endRow > st.startRow {
								startRef, _ := excelize.CoordinatesToCellName(st.startCol, st.startRow)
								endRef, _ := excelize.CoordinatesToCellName(st.startCol+st.span-1, endRow)
								mergeActions = append(mergeActions, mergeAction{start: startRef, end: endRef})
							}
							delete(activeVMerge, startCol)
							return true
						}
					}
					return false
				}

				for closeOverlappedVMerge(cursorCol) {
				}

				// 找到本行下一个可用列（跳过被水平合并占用的列）
				for occupied[fmt.Sprintf("%d,%d", excelRowIdx, cursorCol)] {
					cursorCol++
					for closeOverlappedVMerge(cursorCol) {
					}
				}

				actualCol := cursorCol
				rangeStart, rangeEnd := actualCol, actualCol+span-1

				cellRef, _ := excelize.CoordinatesToCellName(actualCol, excelRowIdx)

				// 标记本行水平占用（跳过起始列）
				for c := actualCol + 1; c <= rangeEnd; c++ {
					occupied[fmt.Sprintf("%d,%d", excelRowIdx, c)] = true
				}

				// 纵向合并起始：记录状态（真正的 MergeCell 在“合并结束时/表格结束时”统一追加）
				if cell.IsVMerge && cell.IsVStart {
					activeVMerge[actualCol] = vMergeState{
						startRow: excelRowIdx,
						startCol: actualCol,
						span:     span,
					}
				}

				// 仅水平合并：立即记录 merge 区间
				// 如果该单元格同时是纵向合并起始，则由纵向合并闭合时生成“矩形合并”，避免重复/重叠合并。
				if span > 1 && !(cell.IsVMerge && cell.IsVStart) {
					endCellRef, _ := excelize.CoordinatesToCellName(rangeEnd, excelRowIdx)
					mergeActions = append(mergeActions, mergeAction{start: cellRef, end: endCellRef})
				}

				cellValue := strings.TrimSpace(cell.Content)
				if err := excelFile.SetCellValue(sheetName, cellRef, cellValue); err != nil {
					return ConversionResult{
						Success: false,
						Message: fmt.Sprintf("写入单元格失败: %v", err),
					}
				}

				if len(cell.Image) > 0 {
					// 目标效果：图片“嵌入式”（随单元格移动并随单元格缩放）
					const (
						targetRowHeight = 60.0 // 约等于 80px（1pt ≈ 1.333px）
						targetColWidth  = 12.0 // 约等于 84px（Excel 默认字体下）
					)

					// 调整行高/列宽，尽量确保图片落在单元格范围内
					// 注意：这里是按“当前图片所在列/行”设置，若同列既有图片又有长文本，可能需要进一步策略化。
					for c := rangeStart; c <= rangeEnd; c++ {
						if colName, err := excelize.ColumnNumberToName(c); err == nil {
							_ = excelFile.SetColWidth(sheetName, colName, colName, targetColWidth)
						}
					}
					_ = excelFile.SetRowHeight(sheetName, excelRowIdx, targetRowHeight)

					imageExt := strings.ToLower(cell.ImageExt)
					if !strings.HasPrefix(imageExt, ".") {
						if imageExt == "" {
							imageExt = "." + detectImageFormat(cell.Image)
						} else {
							imageExt = "." + imageExt
						}
					}

					switch imageExt {
					case ".jpg", ".jpeg":
						imageExt = ".jpeg"
					case ".png":
						imageExt = ".png"
					case ".gif":
						imageExt = ".gif"
					default:
						imageExt = ".png"
					}

					reencodedImage, reencodedExt := reencodeImage(cell.Image, imageExt)

					// vMerge restart 的图片：不用 AutoFit（否则图片会填满整个合并区域），
					// 改用固定缩放，确保图片只显示在 restart 行，不会拉伸至合并区域。
					// 非 vMerge 的图片：用 AutoFit 实现嵌入式随单元格缩放。
					isVMergePic := cell.IsVMerge && cell.IsVStart
					var picOpts excelize.GraphicOptions
					if isVMergePic {
						const (
							targetBoxPx = 80
							cellWidthPx = 84
						)
						const cellHeightPx = 80
						imgW, imgH := imageDimensions(reencodedImage)
						scaleX, scaleY := 1.0, 1.0
						offsetX, offsetY := 0, 0
						if imgW > 0 && imgH > 0 {
							scale := math.Min(float64(targetBoxPx)/float64(imgW), float64(targetBoxPx)/float64(imgH))
							scale = math.Min(1.0, scale) // 只缩小不放大
							scaleX, scaleY = scale, scale
							finalW := float64(imgW) * scale
							finalH := float64(imgH) * scale
							offsetX = int(math.Round((float64(cellWidthPx) - finalW) / 2))
							offsetY = int(math.Round((float64(cellHeightPx) - finalH) / 2))
							if offsetX < 0 {
								offsetX = 0
							}
							if offsetY < 0 {
								offsetY = 0
							}
						}
						picOpts = excelize.GraphicOptions{
							Positioning:     "twoCell",
							LockAspectRatio: true,
							ScaleX:          scaleX,
							ScaleY:          scaleY,
							OffsetX:         offsetX,
							OffsetY:         offsetY,
						}
					} else {
						picOpts = excelize.GraphicOptions{
							Positioning:     "twoCell",
							LockAspectRatio: true,
							AutoFit:         true,
						}
					}
					picture := &excelize.Picture{
						Extension: reencodedExt,
						File:      reencodedImage,
						Format:    &picOpts,
					}
					// 先缓存图片插入动作，等合并单元格完成后再插入。
					// 这样可避免：图片插入后再 merge 导致 WPS/部分客户端重算锚点出现错位。
					pictureActions = append(pictureActions, pictureAction{
						cellRef: cellRef,
						pic:     picture,
					})
				}

				cursorCol = rangeEnd + 1
			}
		}

		// 表格结束后，闭合所有仍然活跃的纵向合并
		lastRow := len(table.Rows)
		for _, st := range activeVMerge {
			if lastRow > st.startRow {
				startRef, _ := excelize.CoordinatesToCellName(st.startCol, st.startRow)
				endRef, _ := excelize.CoordinatesToCellName(st.startCol+st.span-1, lastRow)
				mergeActions = append(mergeActions, mergeAction{start: startRef, end: endRef})
			}
		}

		for _, action := range mergeActions {
			if err := excelFile.MergeCell(sheetName, action.start, action.end); err != nil {
				return ConversionResult{
					Success: false,
					Message: fmt.Sprintf("合并单元格失败: %v", err),
				}
			}
		}

		// 合并完成后再插入图片，降低错位概率
		for _, pa := range pictureActions {
			if err := excelFile.AddPictureFromBytes(sheetName, pa.cellRef, pa.pic); err != nil {
				return ConversionResult{
					Success: false,
					Message: fmt.Sprintf("插入图片失败: %v (单元格: %s)", err, pa.cellRef),
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

// ConvertWordToExcelFromBytes 接收拖拽文件的文件名和字节数据，先写入临时目录再调用转换逻辑。
// 解决浏览器拖拽 API 无法传递完整文件路径的问题。
func (a *App) ConvertWordToExcelFromBytes(fileName string, fileData []byte) ConversionResult {
	if len(fileData) == 0 {
		return ConversionResult{
			Success: false,
			Message: "拖拽的文件数据为空",
		}
	}

	// 写入临时文件
	// 1. 提取纯文件名（如 "用户报告.docx" -> "用户报告"）
	baseName := filepath.Base(fileName)
	oldExt := filepath.Ext(baseName)
	pureName := strings.TrimSuffix(baseName, oldExt)

	// 2. 【核心改动】获取系统临时目录，自己手动拼接路径，不让系统加随机数字！
	// 最终生成的路径会是绝对干净的：/tmp/用户报告.docx
	tmpDir := os.TempDir()
	tmpPath := filepath.Join(tmpDir, pureName+".docx")

	// 3. 直接写入文件
	if err := os.WriteFile(tmpPath, fileData, 0644); err != nil {
		return ConversionResult{
			Success: false,
			Message: fmt.Sprintf("写入临时文件失败: %v", err),
		}
	}

	// 函数执行完后，依然自动把这个干净的临时文件删掉
	defer os.Remove(tmpPath)

	// 4. 此时传过去的文件名是干净的 "用户报告.docx"
	// 你的后面的函数无论是校验 .docx，还是最后替换成 .xlsx，都会是完美的 "用户报告.xlsx"
	return a.ConvertWordToExcel(tmpPath)
}
