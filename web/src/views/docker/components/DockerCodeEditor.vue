<!-- 轻量代码编辑器：透明 textarea 叠加 highlight.js 高亮层 + 行号 -->
<template>
  <div class="docker-code-editor" :style="{ height }">
    <div ref="lineNumbersRef" class="docker-code-editor__lines">
      <div v-for="n in lineCount" :key="n">{{ n }}</div>
    </div>
    <div class="docker-code-editor__body">
      <pre ref="preRef" class="docker-code-editor__highlight" aria-hidden="true"><code v-html="highlighted"></code></pre>
      <textarea
        ref="textareaRef"
        :value="modelValue"
        class="docker-code-editor__input"
        :readonly="readonly"
        spellcheck="false"
        autocomplete="off"
        autocapitalize="off"
        @input="handleInput"
        @scroll="syncScroll"
        @keydown.tab.prevent="insertTab"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
  import { computed, nextTick, ref, watch } from 'vue'
  import hljs from 'highlight.js/lib/core'
  import yaml from 'highlight.js/lib/languages/yaml'
  import bash from 'highlight.js/lib/languages/bash'

  hljs.registerLanguage('yaml', yaml)
  hljs.registerLanguage('bash', bash)

  interface Props {
    modelValue: string
    language?: 'yaml' | 'bash'
    height?: string
    readonly?: boolean
  }
  const props = withDefaults(defineProps<Props>(), {
    language: 'yaml',
    height: '440px',
    readonly: false
  })
  const emit = defineEmits<{ (e: 'update:modelValue', value: string): void }>()

  const textareaRef = ref<HTMLTextAreaElement>()
  const preRef = ref<HTMLElement>()
  const lineNumbersRef = ref<HTMLElement>()

  const lineCount = computed(() => Math.max(1, (props.modelValue || '').split('\n').length))
  const highlighted = computed(() => {
    const code = props.modelValue || ''
    try {
      return hljs.highlight(code, { language: props.language }).value + '\n'
    } catch {
      return escapeHtml(code) + '\n'
    }
  })

  function escapeHtml(text: string) {
    return text
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
  }

  function handleInput(event: Event) {
    const target = event.target as HTMLTextAreaElement
    emit('update:modelValue', target.value)
  }

  function syncScroll() {
    const ta = textareaRef.value
    if (!ta) return
    if (preRef.value) {
      preRef.value.scrollTop = ta.scrollTop
      preRef.value.scrollLeft = ta.scrollLeft
    }
    if (lineNumbersRef.value) {
      lineNumbersRef.value.scrollTop = ta.scrollTop
    }
  }

  function insertTab() {
    const ta = textareaRef.value
    if (!ta || props.readonly) return
    const start = ta.selectionStart
    const end = ta.selectionEnd
    ta.setRangeText('  ', start, end, 'end')
    emit('update:modelValue', ta.value)
    nextTick(syncScroll)
  }

  watch(
    () => props.modelValue,
    () => nextTick(syncScroll)
  )
</script>

<style scoped>
.docker-code-editor {
  display: flex;
  overflow: hidden;
  font-family: 'JetBrains Mono', 'Fira Code', Consolas, Monaco, monospace;
  font-size: 13px;
  line-height: 1.65;
  background: #1e2227;
  border: 1px solid #333a42;
  border-radius: 8px;
}
.docker-code-editor__lines {
  flex: 0 0 46px;
  padding: 12px 8px 12px 0;
  overflow: hidden;
  color: #5b6572;
  text-align: right;
  user-select: none;
  background: #181b20;
}
.docker-code-editor__body {
  position: relative;
  flex: 1;
  min-width: 0;
}
.docker-code-editor__highlight,
.docker-code-editor__input {
  position: absolute;
  inset: 0;
  padding: 12px 14px;
  margin: 0;
  font: inherit;
  line-height: inherit;
  tab-size: 2;
  white-space: pre;
  border: 0;
  outline: none;
}
.docker-code-editor__highlight {
  overflow: hidden;
  color: #c8d3e0;
  pointer-events: none;
}
.docker-code-editor__highlight code {
  font: inherit;
}
.docker-code-editor__input {
  overflow: auto;
  color: transparent;
  resize: none;
  caret-color: #c8d3e0;
  background: transparent;
  -webkit-text-fill-color: transparent;
}
.docker-code-editor__input::selection {
  background: rgb(38 100 180 / 45%);
}
:deep(.hljs-keyword),
:deep(.hljs-selector-tag),
:deep(.hljs-literal) {
  color: #c792ea;
}
:deep(.hljs-attr),
:deep(.hljs-attribute),
:deep(.hljs-property) {
  color: #7fdbca;
}
:deep(.hljs-string),
:deep(.hljs-bullet) {
  color: #ecc48d;
}
:deep(.hljs-number) {
  color: #f78c6c;
}
:deep(.hljs-comment) {
  color: #637777;
  font-style: italic;
}
:deep(.hljs-meta) {
  color: #82aaff;
}
:deep(.hljs-variable),
:deep(.hljs-template-variable) {
  color: #f07178;
}
</style>