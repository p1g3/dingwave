<template>
  <div class="message-panel">
    <div v-if="conversation" class="panel-header">
      <div class="header-left">
        <span class="panel-title">{{ conversation.title }}</span>
        <span class="message-count">{{ conversation.message_count }} 条消息</span>
      </div>
      <div class="search-box">
        <a-dropdown
          v-model:open="showSearchResults"
          :trigger="[]"
          placement="bottomLeft"
        >
          <a-input
            v-model:value="searchKeyword"
            placeholder="在当前会话内搜索消息"
            allow-clear
            @compositionstart="isComposing = true"
            @compositionend="onCompositionEnd"
            @input="onInputChange"
            @clear="clearSearch"
          >
            <template #prefix>
              <SearchOutlined style="color: rgba(128, 128, 128, 0.6)" />
            </template>
          </a-input>
          <template #overlay>
            <div class="search-results-dropdown">
              <a-spin v-if="searchLoading" />
              <EmptyState v-else-if="searchResults.length === 0" message="未找到相关消息" />
              <div
                v-for="msg in searchResults"
                :key="msg.id"
                class="search-result-item"
                @click="onSearchResultClick(msg)"
              >
                <div class="result-sender">{{ msg.sender_name }}</div>
                <div class="result-content" v-html="highlightSearchText(msg.content_text)"></div>
                <div class="result-time">{{ formatMessageTime(msg.created_at) }}</div>
              </div>
              <div v-if="searchTotal > searchResults.length" class="result-more">
                共 {{ searchTotal }} 条结果
              </div>
            </div>
          </template>
        </a-dropdown>
      </div>
    </div>
    <div ref="scrollRef" class="message-list" @scroll="onScroll">
      <a-spin v-if="loading" />
      <div v-for="(msg, index) in messages" :key="msg.id">
        <div v-if="shouldShowTimeSeparator(msg, index)" class="time-separator">
          {{ formatTimeSeparator(msg.created_at) }}
        </div>
        <div :data-message-id="msg.id" :class="['message-row', { mine: isMine(msg), 'target-message': msg.id === targetMessageId }]">
          <a-avatar :size="36" :style="{ backgroundColor: getMessageAvatarColor(isMine(msg)), flexShrink: 0 }">
            {{ msg.sender_name?.[0] || '?' }}
          </a-avatar>
        <div class="message-body">
          <div class="msg-sender">{{ msg.sender_name }}</div>
          <!-- 图片消息 -->
          <template v-if="msg.content_type === 2">
            <div class="image-container">
              <a-image
                v-if="!imageErrors[msg.id]"
                :src="getPrimaryImageUrl(msg)"
                :width="200"
                @error="handleImageError(msg, parseContentJson(msg.content_json))"
              />
              <div v-else class="image-error">
                <FileOutlined />
                <span>{{ parseContentJson(msg.content_json)?.filename || '图片加载失败' }}</span>
              </div>
            </div>
          </template>
          <!-- 文件消息 -->
          <template v-else-if="msg.content_type === 4">
            <a :href="parseContentJson(msg.content_json)?.url" target="_blank" class="file-card">
              <FileOutlined />
              <span class="file-name">{{ parseContentJson(msg.content_json)?.filename }}</span>
              <span class="file-size">{{ formatFileSize(parseContentJson(msg.content_json)?.size) }}</span>
            </a>
          </template>
          <!-- 文本消息 -->
          <template v-else>
            <div class="msg-bubble" v-html="highlightKeywordText(msg.content_text || '[非文本消息]')"></div>
          </template>
          <div class="msg-time">{{ formatMessageTime(msg.created_at) }}</div>
        </div>
        </div>
      </div>
      <EmptyState v-if="!messages.length && !loading" message="暂无消息" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, nextTick, computed } from 'vue'
import { FileOutlined, SearchOutlined } from '@ant-design/icons-vue'
import type { Message, Conversation } from '@/api'
import { searchConversationMessages, highlightText, type SearchMessageItem } from '@/api'
import { useUserStore } from '@/stores/user'
import { formatMessageTime, formatTimeSeparator } from '@/utils/time'
import { getMessageAvatarColor } from '@/utils/avatar'
import { useSearchDebounce } from '@/composables/useSearchDebounce'
import { useScrollPagination } from '@/composables/useScrollPagination'
import EmptyState from './EmptyState.vue'

const props = defineProps<{
  messages: Message[]
  hasMore: boolean
  hasMoreAfter?: boolean
  loading: boolean
  conversation: Conversation | null
  highlightKeyword?: string
  targetMessageId?: number | null
}>()

const emit = defineEmits<{
  loadMore: []
  loadMoreAfter: []
  searchAndJump: [messageId: number, keyword: string, timestamp: number]
  targetScrolled: []
}>()

const userStore = useUserStore()
const scrollRef = ref<HTMLElement>()
const shouldScrollToBottom = ref(true)
const imageErrors = ref<Record<number, boolean>>({})

// Search state
const searchResults = ref<SearchMessageItem[]>([])
const searchLoading = ref(false)
const searchTotal = ref(0)

const doSearch = async (keyword: string) => {
  if (!keyword || !props.conversation) return

  searchLoading.value = true
  try {
    const res = await searchConversationMessages(props.conversation.cid, keyword, 1, 20)
    searchResults.value = res.items
    searchTotal.value = res.total
  } finally {
    searchLoading.value = false
  }
}

const { searchKeyword, isComposing, onInputChange, onCompositionEnd, clearSearch } = useSearchDebounce(doSearch)

const showSearchResults = computed(() => searchKeyword.value.trim().length > 0)

watch(() => props.conversation, () => {
  shouldScrollToBottom.value = true
  clearSearch()
  imageErrors.value = {}
})

watch(() => props.messages, () => {
  if (shouldScrollToBottom.value && props.messages.length) {
    shouldScrollToBottom.value = false
    nextTick(() => {
      if (scrollRef.value) {
        scrollRef.value.scrollTop = scrollRef.value.scrollHeight
      }
    })
  } else if (props.targetMessageId && props.messages.length) {
    // 如果有目标消息ID，滚动到目标位置
    nextTick(() => {
      const el = scrollRef.value?.querySelector(`[data-message-id="${props.targetMessageId}"]`)
      if (el) {
        el.scrollIntoView({ block: 'center' })
        emit('targetScrolled')
      }
    })
  }
})

watch(() => props.targetMessageId, (id) => {
  if (id && props.messages.length) {
    nextTick(() => {
      const el = scrollRef.value?.querySelector(`[data-message-id="${id}"]`)
      if (el) {
        el.scrollIntoView({ block: 'center' })
        emit('targetScrolled')
      }
    })
  }
})

const isMine = (msg: Message) => msg.sender_id === userStore.user?.id

const { onScroll } = useScrollPagination(
  () => emit('loadMore'),
  () => emit('loadMoreAfter')
)

const highlightKeywordText = (text: string) => highlightText(text, props.highlightKeyword || '')

const highlightSearchText = (text: string) => highlightText(text, searchKeyword.value)

const parseContentJson = (json: string) => {
  try {
    return JSON.parse(json)
  } catch {
    return null
  }
}

const formatFileSize = (bytes?: number) => {
  if (!bytes) return ''
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / 1024 / 1024).toFixed(1) + ' MB'
}

const onSearchResultClick = (msg: SearchMessageItem) => {
  emit('searchAndJump', msg.id, searchKeyword.value, msg.created_at)
  clearSearch()
}

const shouldShowTimeSeparator = (msg: Message, index: number) => {
  if (index === 0) return true
  const prevMsg = props.messages[index - 1]
  if (!prevMsg) return false
  const timeDiff = msg.created_at - prevMsg.created_at
  return timeDiff > 1800000
}

const getImageUrl = (content: any): string => {
  if (!content) return ''

  // 尝试使用 resource_cache 中的高质量图片（通过 content_md5）
  try {
    const extension = content.extension ? JSON.parse(content.extension) : null
    if (extension?.content_md5) {
      const md5 = extension.content_md5
      // resource_cache 文件命名格式: md5前2位/md5后位.扩展名
      const dir = md5.substring(0, 2)
      const ext = extension.p_type || 'png'
      return `/cache/${dir}/${md5}.${ext}`
    }
  } catch (e) {
    // JSON 解析失败，继续其他方法
  }

  // 降级使用钉钉云端 URL（高质量图片）
  if (content.url) {
    return content.url
  }

  // 最后降级使用 ImageFiles 中的缩略图
  if (content.mediaId) {
    const mediaId = content.mediaId
    return `/static/${mediaId}.webp`
  }

  return ''
}

const getPrimaryImageUrl = (msg: Message): string => {
  // 优先使用后端映射的本地高质量图
  if ((msg as any).local_image_url) {
    return (msg as any).local_image_url
  }
  // 降级到 content_json 中的字段
  const content = parseContentJson(msg.content_json)
  if (content?.url) return content.url
  if (content?.mediaId) return `/static/${content.mediaId}.webp`
  return ''
}

const handleImageError = (msg: any, content: any) => {
  // 如果主图片加载失败，尝试使用本地缩略图
  // 由于 Vue 的事件处理限制，这里我们直接设置错误标记
  // 未来可以改进为动态切换图片源
  imageErrors.value[msg.id] = true
}

const transformImageUrl = (url: string | undefined): string => {
  if (!url) return ''
  // 直接使用钉钉在线 URL，不替换
  return url
}
</script>

<style scoped>
.message-panel {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.panel-header {
  padding: 12px 16px;
  border-bottom: 1px solid rgba(128, 128, 128, 0.2);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.header-left {
  display: flex;
  align-items: center;
  gap: 12px;
  flex: 1;
  min-width: 0;
}
.panel-title {
  font-weight: 500;
  font-size: 16px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.message-count {
  font-size: 12px;
  color: rgba(128, 128, 128, 0.6);
  white-space: nowrap;
}
.search-box {
  width: 240px;
  flex-shrink: 0;
}
.search-results-dropdown {
  background: white;
  border-radius: 8px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.15);
  max-width: 400px;
  max-height: 400px;
  overflow-y: auto;
  padding: 8px 0;
}
.search-result-item {
  padding: 12px 16px;
  cursor: pointer;
  border-bottom: 1px solid rgba(128, 128, 128, 0.1);
}
.search-result-item:hover {
  background: rgba(128, 128, 128, 0.05);
}
.result-sender {
  font-size: 12px;
  color: rgba(128, 128, 128, 0.8);
  margin-bottom: 4px;
}
.result-content {
  font-size: 14px;
  margin-bottom: 4px;
  word-break: break-word;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}
.result-time {
  font-size: 11px;
  color: rgba(128, 128, 128, 0.6);
}
.result-more {
  padding: 8px 16px;
  text-align: center;
  font-size: 12px;
  color: rgba(128, 128, 128, 0.6);
}
.message-list {
  flex: 1;
  overflow-y: auto;
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.time-separator {
  text-align: center;
  font-size: 12px;
  color: rgba(128, 128, 128, 0.6);
  padding: 12px 0;
}
.loading-tip,
.empty-tip {
  text-align: center;
  color: rgba(128, 128, 128, 0.6);
  padding: 12px;
}
.search-results-dropdown .empty-tip {
  padding: 24px;
}
.message-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
}
.message-row.mine {
  flex-direction: row-reverse;
}
.message-body {
  max-width: 60%;
}
.message-row.mine .message-body {
  text-align: right;
}
.msg-sender {
  font-size: 12px;
  color: rgba(128, 128, 128, 0.8);
  margin-bottom: 4px;
}
.msg-bubble {
  display: inline-block;
  padding: 8px 12px;
  border-radius: 8px;
  background: rgba(128, 128, 128, 0.1);
  word-break: break-word;
  text-align: left;
}
.message-row.mine .msg-bubble {
  background: rgba(22, 119, 255, 0.15);
}
.msg-content {
  word-break: break-word;
}
.msg-time {
  font-size: 11px;
  color: rgba(128, 128, 128, 0.6);
  margin-top: 4px;
  opacity: 0;
  transition: opacity 0.2s;
}
.message-row:hover .msg-time {
  opacity: 1;
}
:deep(.search-highlight) {
  background-color: #ffe58f;
  padding: 0 2px;
  border-radius: 2px;
}
.target-message {
  animation: highlight-fade 2s ease-out;
}
@keyframes highlight-fade {
  0% { background-color: rgba(22, 119, 255, 0.3); }
  100% { background-color: transparent; }
}
.file-card {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px;
  background: rgba(128, 128, 128, 0.1);
  border-radius: 8px;
  text-decoration: none;
  color: inherit;
  max-width: 300px;
}
.file-card:hover {
  background: rgba(128, 128, 128, 0.15);
}
.file-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.file-size {
  font-size: 12px;
  color: rgba(128, 128, 128, 0.6);
  flex-shrink: 0;
}
.image-container {
  display: inline-flex;
  flex-direction: column;
  max-width: 200px;
}
.image-error {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px;
  background: rgba(128, 128, 128, 0.1);
  border-radius: 8px;
  color: rgba(128, 128, 128, 0.8);
  font-size: 14px;
}
</style>
