<template>
  <div class="license-auth">
    <div class="auth-container">
      <div class="auth-header">
        <div class="logo">🔐</div>
        <h1>软件激活</h1>
        <p class="subtitle">Word2Excel 一机一码授权验证</p>
      </div>

      <!-- 加载中 -->
      <div v-if="isChecking" class="checking-section">
        <div class="spinner"></div>
        <p>正在检查激活状态...</p>
      </div>

      <!-- 未激活：显示激活表单 -->
      <div v-else-if="!isActivated" class="activate-section">
        <div class="machine-info">
          <label>本机机器码</label>
          <div class="machine-code-box">
            <code>{{ machineId }}</code>
            <button class="copy-btn" @click="copyMachineId" title="复制机器码">
              📋
            </button>
          </div>
          <p class="hint">请将机器码发送给管理员获取激活码</p>
        </div>

        <div class="activation-form">
          <label>激活码</label>
          <input
            v-model="activationCode"
            type="text"
            placeholder="请输入激活码 (格式: XXXX-XXXX-XXXX-XXXX)"
            :disabled="isActivating"
            @keyup.enter="activate"
          />
        </div>

        <div v-if="errorMessage" class="error-message">
          ❌ {{ errorMessage }}
        </div>

        <button
          class="activate-btn"
          :disabled="isActivating || !activationCode.trim()"
          @click="activate"
        >
          <span v-if="isActivating">
            <span class="btn-spinner"></span>
            激活中...
          </span>
          <span v-else>立即激活</span>
        </button>
      </div>

      <!-- 已激活：显示激活信息 -->
      <div v-else class="activated-section">
        <div class="success-icon">✅</div>
        <h2>软件已激活</h2>
        <p class="success-message">{{ successMessage }}</p>
        <button class="enter-btn" @click="enterApp">
          进入软件
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import { CheckLicense, Activate, GetMachineID } from '../../wailsjs/go/main/App';

interface LicenseCheckResult {
  valid: boolean;
  message: string;
}

interface ActivationResult {
  success: boolean;
  message: string;
}

const emit = defineEmits<{
  (e: 'auth-success'): void;
}>();

const isChecking = ref(true);
const isActivated = ref(false);
const isActivating = ref(false);
const machineId = ref('');
const activationCode = ref('');
const errorMessage = ref('');
const successMessage = ref('');

// 页面加载时检查激活状态
onMounted(async () => {
  try {
    // 获取机器码
    machineId.value = await GetMachineID();

    // 检查许可证
    const result: LicenseCheckResult = await CheckLicense();
    if (result.valid) {
      isActivated.value = true;
      successMessage.value = result.message;
      // 自动进入（可选）
      // setTimeout(() => emit('auth-success'), 1000);
    } else {
      errorMessage.value = result.message;
    }
  } catch (error) {
    console.error('检查激活状态失败:', error);
    errorMessage.value = '检查激活状态失败，请重试';
  } finally {
    isChecking.value = false;
  }
});

// 复制机器码
async function copyMachineId() {
  try {
    await navigator.clipboard.writeText(machineId.value);
    alert('机器码已复制到剪贴板');
  } catch {
    // 降级方案
    const input = document.createElement('input');
    input.value = machineId.value;
    document.body.appendChild(input);
    input.select();
    document.execCommand('copy');
    document.body.removeChild(input);
    alert('机器码已复制到剪贴板');
  }
}

// 激活
async function activate() {
  const code = activationCode.value.trim();
  if (!code) {
    errorMessage.value = '请输入激活码';
    return;
  }

  isActivating.value = true;
  errorMessage.value = '';

  try {
    const result: ActivationResult = await Activate(code);
    if (result.success) {
      isActivated.value = true;
      successMessage.value = result.message;
    } else {
      errorMessage.value = result.message;
    }
  } catch (error) {
    console.error('激活失败:', error);
    errorMessage.value = '激活失败: ' + (error as Error).message;
  } finally {
    isActivating.value = false;
  }
}

// 进入软件
function enterApp() {
  emit('auth-success');
}
</script>

<style scoped>
.license-auth {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
}

.auth-container {
  background: white;
  border-radius: 20px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
  padding: 40px;
  max-width: 500px;
  width: 100%;
  text-align: center;
}

.auth-header {
  margin-bottom: 30px;
}

.logo {
  font-size: 3rem;
  margin-bottom: 10px;
}

.auth-header h1 {
  font-size: 1.8rem;
  color: #333;
  margin-bottom: 5px;
}

.subtitle {
  color: #888;
  font-size: 0.95rem;
}

/* 加载中 */
.checking-section {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 15px;
  padding: 30px 0;
}

.checking-section p {
  color: #666;
}

.spinner {
  width: 40px;
  height: 40px;
  border: 4px solid #f3f3f3;
  border-top: 4px solid #667eea;
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  0% { transform: rotate(0deg); }
  100% { transform: rotate(360deg); }
}

/* 激活表单 */
.activate-section {
  text-align: left;
}

.machine-info {
  margin-bottom: 25px;
}

.machine-info label,
.activation-form label {
  display: block;
  font-size: 0.9rem;
  color: #555;
  margin-bottom: 8px;
  font-weight: 500;
}

.machine-code-box {
  display: flex;
  align-items: center;
  gap: 10px;
  background: #f5f5f5;
  border-radius: 10px;
  padding: 12px 15px;
  border: 1px solid #e0e0e0;
}

.machine-code-box code {
  flex: 1;
  font-family: 'Courier New', monospace;
  font-size: 0.95rem;
  color: #333;
  word-break: break-all;
}

.copy-btn {
  background: none;
  border: none;
  font-size: 1.2rem;
  cursor: pointer;
  padding: 5px;
  border-radius: 5px;
  transition: background 0.2s;
}

.copy-btn:hover {
  background: #e0e0e0;
}

.hint {
  font-size: 0.8rem;
  color: #999;
  margin-top: 8px;
  text-align: center;
}

.activation-form {
  margin-bottom: 20px;
}

.activation-form input {
  width: 100%;
  padding: 14px 16px;
  border: 2px solid #e0e0e0;
  border-radius: 10px;
  font-size: 1rem;
  transition: border-color 0.3s;
  box-sizing: border-box;
}

.activation-form input:focus {
  outline: none;
  border-color: #667eea;
}

.activation-form input:disabled {
  background: #f5f5f5;
  cursor: not-allowed;
}

.error-message {
  background: #f8d7da;
  border: 1px solid #f5c6cb;
  color: #721c24;
  padding: 12px 16px;
  border-radius: 8px;
  margin-bottom: 20px;
  font-size: 0.9rem;
}

.activate-btn {
  width: 100%;
  padding: 15px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
  border: none;
  border-radius: 10px;
  font-size: 1.1rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.3s ease;
  box-shadow: 0 4px 15px rgba(102, 126, 234, 0.4);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
}

.activate-btn:hover:not(:disabled) {
  transform: translateY(-2px);
  box-shadow: 0 6px 20px rgba(102, 126, 234, 0.6);
}

.activate-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.btn-spinner {
  display: inline-block;
  width: 16px;
  height: 16px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top: 2px solid white;
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

/* 已激活 */
.activated-section {
  padding: 20px 0;
}

.success-icon {
  font-size: 4rem;
  margin-bottom: 15px;
}

.activated-section h2 {
  color: #28a745;
  margin-bottom: 10px;
}

.success-message {
  color: #666;
  margin-bottom: 25px;
}

.enter-btn {
  padding: 15px 50px;
  background: linear-gradient(135deg, #28a745 0%, #20c997 100%);
  color: white;
  border: none;
  border-radius: 10px;
  font-size: 1.1rem;
  font-weight: 600;
  cursor: pointer;
  transition: all 0.3s ease;
  box-shadow: 0 4px 15px rgba(40, 167, 69, 0.4);
}

.enter-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 6px 20px rgba(40, 167, 69, 0.6);
}
</style>
