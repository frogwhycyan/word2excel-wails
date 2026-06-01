<template>
  <div id="app">
    <div class="container">
      <header>
        <h1>Word 转 Excel</h1>
        <p class="subtitle">将 Word 文档中的表格快速转换为 Excel 文件</p>
      </header>

      <div
        class="drop-zone"
        :class="{ 'drag-over': isDragOver, 'has-file': selectedFile }"
        @dragover.prevent="onDragOver"
        @dragleave="onDragLeave"
        @drop.prevent="onDrop"
        @click="selectFile"
      >
        <div v-if="!selectedFile && !isProcessing" class="drop-zone-content">
          <div class="icon">📄</div>
          <p class="main-text">点击选择 Word 文件 或 拖拽文件到此处</p>
          <p class="sub-text">支持 .docx 格式</p>
        </div>

        <div v-if="selectedFile && !isProcessing" class="file-info">
          <div class="icon">✅</div>
          <p class="file-name">{{ fileName }}</p>
          <button class="change-btn" @click.stop="selectFile">更换文件</button>
        </div>

        <div v-if="isProcessing" class="processing">
          <div class="spinner"></div>
          <p>正在解析并转换中...</p>
        </div>
      </div>

      <div v-if="resultMessage" class="result" :class="{ 'success': isSuccess, 'error': !isSuccess }">
        <div class="result-icon">{{ isSuccess ? '✅' : '❌' }}</div>
        <p class="result-message">{{ resultMessage }}</p>
        <p v-if="isSuccess && excelPath" class="result-path">保存位置: {{ excelPath }}</p>
      </div>

      <div v-if="selectedFile && !isProcessing && !resultMessage" class="action">
        <button class="convert-btn" @click="convertFile">
          开始转换
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import {ref, computed} from 'vue';
import {OpenWordFile, ConvertWordToExcel, ProcessDroppedFile} from '../wailsjs/go/main/App';

interface FileDialogResult {
  success: boolean;
  filePath: string;
  message: string;
}

interface ConversionResult {
  success: boolean;
  message: string;
  tableCount: number;
  excelPath: string;
}

const selectedFile = ref('');
const isDragOver = ref(false);
const isProcessing = ref(false);
const resultMessage = ref('');
const isSuccess = ref(false);
const excelPath = ref('');

const fileName = computed(() => {
  if (!selectedFile.value) return '';
  const parts = selectedFile.value.split(/[/\\]/);
  return parts[parts.length - 1];
});

function onDragOver(event: DragEvent) {
  isDragOver.value = true;
}

function onDragLeave(event: DragEvent) {
  isDragOver.value = false;
}

async function onDrop(event: DragEvent) {
  isDragOver.value = false;

  const files = event.dataTransfer?.files;
  if (!files || files.length === 0) {
    return;
  }

  const file = files[0];
  if (!file.name.toLowerCase().endsWith('.docx')) {
    resultMessage.value = '仅支持 .docx 格式的 Word 文档';
    isSuccess.value = false;
    return;
  }

  selectedFile.value = file.path || file.name;
  resultMessage.value = '';

  await convertFile();
}

async function selectFile() {
  if (isProcessing.value) return;

  try {
    const result: FileDialogResult = await OpenWordFile();
    if (result.success) {
      selectedFile.value = result.filePath;
      resultMessage.value = '';
    } else if (result.message && result.message !== '未选择文件') {
      resultMessage.value = result.message;
      isSuccess.value = false;
    }
  } catch (error) {
    console.error('选择文件失败:', error);
    resultMessage.value = '选择文件失败';
    isSuccess.value = false;
  }
}

async function convertFile() {
  if (!selectedFile.value || isProcessing.value) return;

  isProcessing.value = true;
  resultMessage.value = '';

  try {
    const result: ConversionResult = await ConvertWordToExcel(selectedFile.value);
    isSuccess.value = result.success;
    resultMessage.value = result.message;
    excelPath.value = result.excelPath || '';
  } catch (error) {
    console.error('转换失败:', error);
    isSuccess.value = false;
    resultMessage.value = '转换失败: ' + (error as Error).message;
  } finally {
    isProcessing.value = false;
  }
}
</script>

<style scoped>
* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

#app {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
}

.container {
  background: white;
  border-radius: 20px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
  padding: 40px;
  max-width: 600px;
  width: 100%;
}

header {
  text-align: center;
  margin-bottom: 40px;
}

h1 {
  font-size: 2.5rem;
  color: #333;
  margin-bottom: 10px;
}

.subtitle {
  color: #666;
  font-size: 1.1rem;
}

.drop-zone {
  border: 3px dashed #ddd;
  border-radius: 15px;
  padding: 60px 40px;
  text-align: center;
  cursor: pointer;
  transition: all 0.3s ease;
  background: #f9f9f9;
}

.drop-zone:hover {
  border-color: #667eea;
  background: #f0f0ff;
}

.drop-zone.drag-over {
  border-color: #667eea;
  background: #e8e8ff;
  transform: scale(1.02);
}

.drop-zone.has-file {
  border-color: #667eea;
  background: #f0f0ff;
}

.drop-zone-content,
.file-info,
.processing {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 15px;
}

.icon {
  font-size: 4rem;
}

.main-text {
  font-size: 1.3rem;
  color: #333;
  font-weight: 500;
}

.sub-text {
  color: #888;
  font-size: 0.95rem;
}

.file-name {
  font-size: 1.2rem;
  color: #667eea;
  font-weight: 600;
  word-break: break-all;
}

.change-btn {
  padding: 8px 20px;
  background: white;
  border: 2px solid #667eea;
  border-radius: 8px;
  color: #667eea;
  font-size: 0.95rem;
  cursor: pointer;
  transition: all 0.3s ease;
}

.change-btn:hover {
  background: #667eea;
  color: white;
}

.processing {
  color: #667eea;
}

.spinner {
  width: 50px;
  height: 50px;
  border: 4px solid #f3f3f3;
  border-top: 4px solid #667eea;
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  0% { transform: rotate(0deg); }
  100% { transform: rotate(360deg); }
}

.result {
  margin-top: 30px;
  padding: 20px;
  border-radius: 12px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  animation: slideIn 0.3s ease;
}

.result.success {
  background: #d4edda;
  border: 2px solid #28a745;
}

.result.error {
  background: #f8d7da;
  border: 2px solid #dc3545;
}

.result-icon {
  font-size: 2.5rem;
}

.result-message {
  font-size: 1.1rem;
  color: #333;
  text-align: center;
}

.result-path {
  font-size: 0.9rem;
  color: #666;
  word-break: break-all;
  text-align: center;
}

.action {
  margin-top: 30px;
  text-align: center;
}

.convert-btn {
  padding: 15px 50px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
  border: none;
  border-radius: 10px;
  font-size: 1.2rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.3s ease;
  box-shadow: 0 4px 15px rgba(102, 126, 234, 0.4);
}

.convert-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 6px 20px rgba(102, 126, 234, 0.6);
}

.convert-btn:active {
  transform: translateY(0);
}

@keyframes slideIn {
  from {
    opacity: 0;
    transform: translateY(-10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
