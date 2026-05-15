<template>
  <div class="snow-page">
    <a-spin :loading="loading" tip="正在加载..." class="full-height">
      <!-- 警告横幅 -->
      <a-alert type="warning" show-icon class="warning-banner">
        <template #icon>
          <icon-exclamation-circle />
        </template>
        <div>
          <strong>重要提醒：</strong>
          一般情况下不推荐修改RPC节点,除非您非常了解区块网络并确保节点的可用性和稳定性。
        </div>
      </a-alert>

      <!-- 主要内容 -->
      <a-card :bordered="false" class="main-card">
        <template #title>
          <div class="card-title">
            <div class="title-icon">
              <icon-settings />
            </div>
            <span>区块网络配置</span>
          </div>
        </template>

        <template #extra>
          <a-space size="small" wrap>
            <a-button @click="handleReset" :loading="loading" size="small" class="action-btn">
              <template #icon>
                <icon-refresh />
              </template>
              重置
            </a-button>
            <a-button type="primary" @click="handleSave" :loading="saveLoading" size="small" class="action-btn save-btn">
              <template #icon>
                <icon-save />
              </template>
              保存配置
            </a-button>
          </a-space>
        </template>

        <div class="form-container">
          <a-form :model="formData" layout="vertical" ref="formRef">
            <!-- Tron 网络配置区域 -->
            <div class="tron-section">
              <div class="section-header">
                <div class="header-icon">
                  <icon-fire />
                </div>
                <span class="header-title">Tron 网络</span>
              </div>

              <a-row :gutter="16">
                <a-col :xs="24" :sm="24" :md="12">
                  <a-form-item
                    field="rpc_endpoint_tron"
                    :rules="[{ required: true, message: '请输入Tron RPC' }]"
                    class="network-form-item"
                  >
                    <template #label>
                      <div class="tron-grid-label">
                        <span class="label-with-tip">
                          <span>Tron RPC</span>
                          <a-tooltip content="支持多个节点，每行一个，自动轮询并故障切换" position="top">
                            <icon-question-circle class="tip-icon" />
                          </a-tooltip>
                        </span>
                      </div>
                    </template>
                    <a-textarea
                      v-model="formData.rpc_endpoint_tron"
                      placeholder="请输入 Tron RPC，多个节点每行一个"
                      allow-clear
                      size="small"
                      class="network-input tron-input"
                      :auto-size="{ minRows: 1, maxRows: 6 }"
                    />
                  </a-form-item>
                </a-col>
                <a-col :span="24">
                  <a-form-item field="rpc_endpoint_tron_grid_api_key" class="network-form-item">
                    <template #label>
                      <div class="tron-grid-label">
                        <span class="label-with-tip">
                          <span>Tron Grid Api Key</span>
                          <a-tooltip content="配置独立 Api Key 可提高扫块稳定性，多个可用半角符逗号隔开。" position="top">
                            <icon-question-circle class="tip-icon" />
                          </a-tooltip>
                          <span class="optional-tag">(可选)</span>
                        </span>
                        <a
                          href="https://github.com/v03413/BEpusdt/blob/main/docs/tron-grid/readme.md"
                          target="_blank"
                          class="help-link"
                        >
                          <icon-question-circle />
                          获取方法
                        </a>
                      </div>
                    </template>

                    <a-textarea
                      v-model="formData.rpc_endpoint_tron_grid_api_key"
                      placeholder="请输入 Tron Grid Api Key (可选)，多个可用半角符逗号隔开"
                      allow-clear
                      size="small"
                      class="network-input tron-input tron-grid-api-key-input"
                      :auto-size="{ minRows: 1, maxRows: 6 }"
                    >
                      <template>
                        <div class="input-icon">
                          <icon-safe />
                        </div>
                      </template>
                    </a-textarea>
                  </a-form-item>
                </a-col>
              </a-row>
            </div>

            <!-- 其他网络配置 -->
            <div class="other-section">
              <div class="section-header">
                <div class="header-icon">
                  <icon-link />
                </div>
                <span class="header-title">其他网络</span>
              </div>

              <a-row :gutter="[16, 6]">
                <a-col
                  v-for="network in networks.filter(n => n.key !== 'rpc_endpoint_tron')"
                  :key="network.key"
                  :xs="24"
                  :sm="24"
                  :md="12"
                  :lg="8"
                >
                  <a-form-item
                    :field="network.key"
                    :rules="[{ required: true, message: `请输入${network.label}` }]"
                    class="network-form-item"
                  >
                    <template #label>
                      <div class="tron-grid-label">
                        <span class="label-with-tip">
                          <span>{{ network.label }}</span>
                          <a-tooltip content="支持多个节点，每行一个，自动轮询并故障切换" position="top">
                            <icon-question-circle class="tip-icon" />
                          </a-tooltip>
                        </span>
                      </div>
                    </template>
                    <a-textarea
                      v-model="formData[network.key]"
                      :placeholder="`请输入 ${network.label}，多个节点每行一个`"
                      allow-clear
                      size="small"
                      class="network-input"
                      :auto-size="{ minRows: 1, maxRows: 6 }"
                    />
                  </a-form-item>
                </a-col>
              </a-row>
            </div>

            <!-- BSC 扫块高级参数 -->
            <div class="evm-tuning-section">
              <div class="section-header">
                <div class="header-icon">
                  <icon-thunderbolt />
                </div>
                <span class="header-title">BSC 扫块高级参数</span>
                <span class="section-tip">仅作用于 BSC，公网 RPC 节点出现限流时建议调小</span>
              </div>

              <a-row :gutter="[16, 6]">
                <a-col :xs="24" :sm="12" :md="6">
                  <a-form-item
                    field="evm_block_parse_max_num_bsc"
                    :rules="[{ required: true, type: 'number', positive: true, message: '请输入正整数' }]"
                    class="network-form-item"
                  >
                    <template #label>
                      <div class="tron-grid-label">
                        <span class="label-with-tip">
                          <span>BSC getBlock 单批数</span>
                          <a-tooltip
                            content="单次 eth_getBlockByNumber 数组的区块数量。BSC Parse=true 时响应体大、易超时，公共节点建议小（默认 5）；自建/付费节点稳定可调大提高吞吐。"
                            position="top"
                          >
                            <icon-question-circle class="tip-icon" />
                          </a-tooltip>
                        </span>
                      </div>
                    </template>
                    <a-input
                      v-model="formData.evm_block_parse_max_num_bsc"
                      placeholder="默认 5"
                      allow-clear
                      size="small"
                      class="network-input"
                    />
                  </a-form-item>
                </a-col>
                <a-col :xs="24" :sm="12" :md="6">
                  <a-form-item
                    field="evm_block_logs_max_num_bsc"
                    :rules="[{ required: true, type: 'number', positive: true, message: '请输入正整数' }]"
                    class="network-form-item"
                  >
                    <template #label>
                      <div class="tron-grid-label">
                        <span class="label-with-tip">
                          <span>BSC getLogs 单次块数</span>
                          <a-tooltip
                            content="单次 eth_getLogs 调用覆盖的区块数。公共节点上限通常 5000，比 getBlock 宽松得多。调大可显著减少请求次数（默认 50）。过大可能在 USDT 等热门合约上响应体过大被节点拒绝。"
                            position="top"
                          >
                            <icon-question-circle class="tip-icon" />
                          </a-tooltip>
                        </span>
                      </div>
                    </template>
                    <a-input
                      v-model="formData.evm_block_logs_max_num_bsc"
                      placeholder="默认 50"
                      allow-clear
                      size="small"
                      class="network-input"
                    />
                  </a-form-item>
                </a-col>
                <a-col :xs="24" :sm="12" :md="6">
                  <a-form-item
                    field="evm_block_dispatch_pool_bsc"
                    :rules="[{ required: true, type: 'number', positive: true, message: '请输入正整数' }]"
                    class="network-form-item"
                  >
                    <template #label>
                      <div class="tron-grid-label">
                        <span class="label-with-tip">
                          <span>BSC 区块消费并发数</span>
                          <a-tooltip
                            content="BSC 区块解析的并发 worker 数量。公共节点限流明显时建议设为 1；自建节点可加大。修改后需要重启服务生效。默认 3。"
                            position="top"
                          >
                            <icon-question-circle class="tip-icon" />
                          </a-tooltip>
                        </span>
                      </div>
                    </template>
                    <a-input
                      v-model="formData.evm_block_dispatch_pool_bsc"
                      placeholder="默认 3"
                      allow-clear
                      size="small"
                      class="network-input"
                    />
                  </a-form-item>
                </a-col>
                <a-col :xs="24" :sm="12" :md="6">
                  <a-form-item
                    field="evm_block_roll_offset_bsc"
                    :rules="[
                      { required: true, message: '请输入非负整数' },
                      { match: /^\d+$/, message: '只能输入非负整数' }
                    ]"
                    class="network-form-item"
                  >
                    <template #label>
                      <div class="tron-grid-label">
                        <span class="label-with-tip">
                          <span>BSC 扫块滞后块数</span>
                          <a-tooltip
                            content="每轮拉取最新区块高度后，再向后退几个块作为扫描终点。某些公共节点对最新区块有同步延迟，直接扫会报 block is out of range 或返回空，适当滞后可降低失败率。默认 5；自建节点稳定可设为 0。"
                            position="top"
                          >
                            <icon-question-circle class="tip-icon" />
                          </a-tooltip>
                        </span>
                      </div>
                    </template>
                    <a-input
                      v-model="formData.evm_block_roll_offset_bsc"
                      placeholder="默认 5"
                      allow-clear
                      size="small"
                      class="network-input"
                    />
                  </a-form-item>
                </a-col>
              </a-row>
            </div>
          </a-form>
        </div>

        <!-- 配置说明 -->
        <a-divider orientation="left" class="info-divider">
          <div class="divider-content">
            <icon-info-circle />
            <span>配置说明</span>
          </div>
        </a-divider>

        <div class="info-section">
          <div class="info-grid">
            <div v-for="(info, index) in infoList" :key="index" class="info-item">
              <div class="info-icon">
                <component :is="info.icon" />
              </div>
              <span>{{ info.text }}</span>
            </div>
          </div>
        </div>
      </a-card>
    </a-spin>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from "vue";
import { Message } from "@arco-design/web-vue";
import { getsConfAPI, setsConfAPI } from "@/api/modules/conf/index";
import {
  IconSettings,
  IconSave,
  IconRefresh,
  IconExclamationCircle,
  IconLink,
  IconInfoCircle,
  IconCheckCircle,
  IconStar,
  IconThunderbolt,
  IconFire,
  IconQuestionCircle,
  IconSafe
} from "@arco-design/web-vue/es/icon";

// 网络配置
const networks = [
  { key: "rpc_endpoint_ethereum", label: "Ethereum RPC", icon: IconLink },
  { key: "rpc_endpoint_bsc", label: "BSC RPC", icon: IconLink },
  { key: "rpc_endpoint_polygon", label: "Polygon RPC", icon: IconLink },
  { key: "rpc_endpoint_arbitrum", label: "Arbitrum RPC", icon: IconLink },
  { key: "rpc_endpoint_base", label: "Base RPC", icon: IconLink },
  { key: "rpc_endpoint_xlayer", label: "X Layer RPC", icon: IconLink },
  { key: "rpc_endpoint_tron", label: "Tron RPC", icon: IconLink },
  { key: "rpc_endpoint_solana", label: "Solana RPC", icon: IconLink },
  { key: "rpc_endpoint_aptos", label: "Aptos RPC", icon: IconLink },
  { key: "rpc_endpoint_plasma", label: "Plasma RPC", icon: IconLink }
];

const infoList = [
  { icon: IconCheckCircle, text: "RPC节点是与区块链网络通信的关键接口，请确保所配置的节点稳定可靠" },
  { icon: IconStar, text: "每个网络支持多个 RPC 节点，每行一个；系统会自动轮询并在节点限流或异常时临时切换到其他节点" },
  { icon: IconThunderbolt, text: "配置前请先测试节点的连通性和响应速度" },
  { icon: IconFire, text: "修改配置后系统将立即生效，请谨慎操作" }
];

const loading = ref<boolean>(false);
const saveLoading = ref<boolean>(false);
const formRef = ref();
const formData = reactive<Record<string, string>>({});
const originalData = ref<Record<string, string>>({});

const getConf = async () => {
  try {
    loading.value = true;
    const keys = [
      ...networks.map(network => network.key),
      "rpc_endpoint_tron_grid_api_key",
      "evm_block_parse_max_num_bsc",
      "evm_block_logs_max_num_bsc",
      "evm_block_dispatch_pool_bsc",
      "evm_block_roll_offset_bsc"
    ];

    const response = await getsConfAPI({ keys });
    const data = response.data || {};

    networks.forEach(network => {
      formData[network.key] = data[network.key] || "";
    });
    formData.rpc_endpoint_tron_grid_api_key = data.rpc_endpoint_tron_grid_api_key || "";
    formData.evm_block_parse_max_num_bsc = data.evm_block_parse_max_num_bsc || "5";
    formData.evm_block_logs_max_num_bsc = data.evm_block_logs_max_num_bsc || "50";
    formData.evm_block_dispatch_pool_bsc = data.evm_block_dispatch_pool_bsc || "3";
    formData.evm_block_roll_offset_bsc = data.evm_block_roll_offset_bsc ?? "5";

    originalData.value = { ...formData };
  } catch (error) {
    Message.error("获取配置失败");
    console.error("获取配置失败:", error);
  } finally {
    loading.value = false;
  }
};

const handleSave = async () => {
  try {
    const errors = await formRef.value?.validate();
    if (errors) {
      Message.error("表单验证失败，请检查所有字段");
      return;
    }
  } catch (validationError) {
    console.error("表单验证失败:", validationError);
    Message.error("请填写所有必填项");
    return;
  }

  try {
    saveLoading.value = true;

    // 构建保存数据数组
    const saveData: Array<{ key: string; value: string }> = [];

    // 添加所有网络的 RPC 配置
    networks.forEach(network => {
      const value = formData[network.key]?.trim();
      if (value) {
        saveData.push({
          key: network.key,
          value: value
        });
      }
    });

    // 验证所有必填的 RPC 节点是否都已填写
    if (saveData.length < networks.length) {
      Message.error("所有RPC节点都必须填写");
      return;
    }

    // 添加 Tron Grid Api Key (可选，但即使为空也要保存)
    const tronApiKey = formData.rpc_endpoint_tron_grid_api_key?.trim() || "";
    saveData.push({
      key: "rpc_endpoint_tron_grid_api_key",
      value: tronApiKey
    });

    // BSC 扫块高级参数
    saveData.push({
      key: "evm_block_parse_max_num_bsc",
      value: formData.evm_block_parse_max_num_bsc?.trim() || "5"
    });
    saveData.push({
      key: "evm_block_logs_max_num_bsc",
      value: formData.evm_block_logs_max_num_bsc?.trim() || "50"
    });
    saveData.push({
      key: "evm_block_dispatch_pool_bsc",
      value: formData.evm_block_dispatch_pool_bsc?.trim() || "3"
    });
    saveData.push({
      key: "evm_block_roll_offset_bsc",
      value: formData.evm_block_roll_offset_bsc?.trim() ?? "5"
    });

    await setsConfAPI(saveData);

    Message.success("配置保存成功");

    await getConf();
  } catch (error) {
    Message.error("保存配置失败");
    console.error("保存配置失败:", error);
  } finally {
    saveLoading.value = false;
  }
};

// 重置配置
const handleReset = () => {
  Object.assign(formData, originalData.value);
  Message.info("已重置为原始配置");
};

onMounted(() => {
  getConf();
});
</script>

<style lang="scss" scoped>
.full-height {
  min-height: 100%;
}

.warning-banner {
  margin-bottom: 12px;
  border-radius: 6px;
  box-shadow: 0 2px 6px rgb(var(--warning-6) / 8%);
  background: rgb(var(--warning-1));
  border: 1px solid rgb(var(--warning-3));

  :deep(.arco-alert-content) {
    font-size: 13px;
    line-height: 1.4;
  }

  :deep(.arco-alert) {
    padding: 10px 14px;
  }
}

.main-card {
  margin-top: 0;
  border-radius: 8px;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.04);
  background: $color-bg-2;

  :deep(.arco-card-header) {
    border-bottom: 1px solid $color-border-2;
    padding: 14px 18px;
    background: $color-bg-3;
    border-radius: 8px 8px 0 0;
  }

  :deep(.arco-card-body) {
    padding: 16px;
  }
}

.card-title {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  font-size: 15px;
  color: $color-text-1;

  .title-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    background: $color-primary;
    border-radius: 6px;
    color: #fff;
    font-size: 13px;
  }
}

.action-btn {
  border-radius: 6px;
  font-weight: 500;
  transition: all 0.2s ease;
  padding: 5px 14px;
  height: 30px;

  &:hover {
    transform: translateY(-1px);
    box-shadow: 0 3px 8px rgba(0, 0, 0, 0.12);
  }
}

.save-btn {
  background: $color-primary;
  border: none;

  &:hover {
    background: rgb(var(--primary-5));
  }
}

.form-container {
  margin: 12px 0;
}

// Tron 配置区域样式 - 使用 Tron 官方红色系
.tron-section {
  background: rgba(var(--danger-6), 0.06);
  border: 1px solid rgba(var(--danger-6), 0.18);
  border-radius: 6px;
  padding: 10px 12px;
  margin-bottom: 12px;
  position: relative;
  overflow: hidden;

  &::before {
    content: "";
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 2px;
    background: rgba(var(--danger-6), 0.72);
  }

  .section-header {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 8px;
    padding-bottom: 6px;
    border-bottom: 1px solid $color-border-2;

    .header-icon {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 20px;
      height: 20px;
      background: $color-danger;
      border-radius: 4px;
      color: #fff;
      font-size: 11px;
      box-shadow: 0 2px 4px rgba(var(--danger-6), 0.3);
    }

    .header-title {
      font-weight: 600;
      font-size: 13px;
      color: $color-text-1;
    }
  }
}

// EVM 扫块高级参数区域样式 - 使用柔和的蓝色系
.evm-tuning-section {
  background: rgba(var(--primary-6), 0.05);
  border: 1px solid rgba(var(--primary-6), 0.18);
  border-radius: 6px;
  padding: 10px 12px;
  margin-bottom: 12px;
  position: relative;
  overflow: hidden;

  &::before {
    content: "";
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 2px;
    background: rgba(var(--primary-6), 0.72);
  }

  .section-header {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 8px;
    padding-bottom: 6px;
    border-bottom: 1px solid $color-border-2;

    .header-icon {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 20px;
      height: 20px;
      background: $color-primary;
      border-radius: 4px;
      color: #fff;
      font-size: 11px;
      box-shadow: 0 2px 4px rgba(var(--primary-6), 0.3);
    }

    .header-title {
      font-weight: 600;
      font-size: 13px;
      color: $color-text-1;
    }

    .section-tip {
      font-size: 11px;
      font-weight: normal;
      color: $color-text-3;
      margin-left: auto;
    }
  }
}

// 其他网络配置区域样式 - 使用柔和的浅绿色系
.other-section {
  background: rgba(var(--success-6), 0.06);
  border: 1px solid rgba(var(--success-6), 0.18);
  border-radius: 6px;
  padding: 10px 12px;
  margin-bottom: 12px;
  position: relative;
  overflow: hidden;

  &::before {
    content: "";
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 2px;
    background: rgba(var(--success-6), 0.72);
  }

  .section-header {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-bottom: 8px;
    padding-bottom: 6px;
    border-bottom: 1px solid $color-border-2;

    .header-icon {
      display: flex;
      align-items: center;
      justify-content: center;
      width: 20px;
      height: 20px;
      background: $color-success;
      border-radius: 4px;
      color: #fff;
      font-size: 11px;
      box-shadow: 0 2px 4px rgba(var(--success-6), 0.3);
    }

    .header-title {
      font-weight: 600;
      font-size: 13px;
      color: $color-text-1;
    }
  }

  .network-input {
    :deep(.arco-input-wrapper) {
      border-color: $color-border-2;
      background: $color-bg-2;

      &:hover {
        border-color: $color-success;
        box-shadow: 0 0 0 2px rgba(var(--success-6), 0.08);
      }

      &.arco-input-focus {
        border-color: $color-success;
        box-shadow: 0 0 0 2px rgba(var(--success-6), 0.1);
      }
    }
  }
}

.network-form-item {
  :deep(.arco-form-item-label-col) {
    margin-bottom: 4px;

    .arco-form-item-label {
      font-weight: 500;
      color: $color-text-1;
      font-size: 12px;
    }
  }
}

.network-input {
  border-radius: 6px;
  transition: all 0.2s ease;

  :deep(.arco-input-wrapper) {
    border: 1px solid $color-border-2;
    background: $color-bg-2;
    height: 32px;

    &:hover {
      border-color: $color-primary;
      box-shadow: 0 0 0 2px rgba(var(--primary-6), 0.08);
    }

    &.arco-input-focus {
      border-color: $color-primary;
      box-shadow: 0 0 0 2px rgba(var(--primary-6), 0.1);
    }
  }

  :deep(.arco-input) {
    font-size: 12px;
  }
}

// Tron 输入框特殊样式
.tron-input {
  :deep(.arco-input-wrapper) {
    border-color: $color-border-2;

    &:hover {
      border-color: $color-danger;
      box-shadow: 0 0 0 2px rgba(var(--danger-6), 0.08);
    }

    &.arco-input-focus {
      border-color: $color-danger;
      box-shadow: 0 0 0 2px rgba(var(--danger-6), 0.1);
    }
  }
}

.tron-grid-api-key-input {
  max-width: 100%;

  :deep(textarea) {
    max-height: 120px;
    overflow-y: auto;
    line-height: 20px;
  }
}

.input-icon {
  display: flex;
  align-items: center;
  color: $color-text-3;
  font-size: 13px;
}

.tron-grid-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  max-width: 100%;
  min-width: 0;
  flex-wrap: wrap;
}

.label-with-tip {
  display: flex;
  align-items: center;
  gap: 5px;

  .tip-icon {
    color: $color-text-3;
    cursor: help;
    font-size: 13px;

    &:hover {
      color: $color-primary;
    }
  }

  .optional-tag {
    color: $color-text-3;
    font-size: 11px;
    font-weight: normal;
  }
}

.help-link {
  display: flex;
  align-items: center;
  gap: 3px;
  color: $color-danger;
  font-size: 11px;
  text-decoration: none;
  transition: all 0.2s ease;
  font-weight: 500;

  &:hover {
    color: rgb(var(--danger-5));
  }
}

.info-divider {
  margin: 16px 0 12px 0;

  .divider-content {
    display: flex;
    align-items: center;
    gap: 5px;
    color: $color-text-1;
    font-weight: 500;
    font-size: 13px;
  }

  :deep(.arco-divider-text) {
    background: $color-bg-2;
    border: 1px solid $color-border-2;
    border-radius: 12px;
    padding: 4px 10px;
    font-size: 12px;
  }
}

.info-section {
  background: $color-bg-3;
  border-radius: 6px;
  padding: 12px;
  border: 1px solid $color-border-2;
}

.info-grid {
  display: grid;
  gap: 8px;
}

.info-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 7px;
  background: $color-bg-2;
  border-radius: 5px;
  border: 1px solid $color-border-2;
  transition: all 0.2s ease;

  &:hover {
    transform: translateY(-1px);
    box-shadow: 0 2px 6px rgba(0, 0, 0, 0.06);
    border-color: $color-primary;
  }

  .info-icon {
    flex-shrink: 0;
    width: 16px;
    height: 16px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: $color-primary;
    font-size: 13px;
    margin-top: 1px;
  }

  span {
    color: $color-text-2;
    line-height: 1.4;
    font-size: 12px;
  }
}

// 响应式设计
@media (max-width: 768px) {
  .card-title {
    font-size: 14px;

    .title-icon {
      width: 26px;
      height: 26px;
      font-size: 12px;
    }
  }

  .info-grid {
    grid-template-columns: 1fr;
  }

  .main-card {
    :deep(.arco-card-header) {
      padding: 12px 14px;
    }

    :deep(.arco-card-body) {
      padding: 14px;
    }
  }

  .tron-section,
  .other-section {
    padding: 8px 10px;
  }
}

// 暗色主题适配
:deep(.arco-card.arco-card-bordered) {
  border: 1px solid $color-border-2;
}
</style>
