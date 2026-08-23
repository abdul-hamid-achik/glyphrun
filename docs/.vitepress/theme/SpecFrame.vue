<script setup lang="ts">
defineProps<{
  file: string
}>()
</script>

<template>
  <figure class="gr-spec">
    <figcaption class="gr-spec__bar">
      <span class="gr-spec__file">{{ file }}</span>
    </figcaption>
    <div class="gr-spec__body">
      <slot />
    </div>
  </figure>
</template>
