// Copyright (C) 2023 Storj Labs, Inc.
// See LICENSE for copying information.

<template>
  <v-card :title="title" variant="flat" :border="true" rounded="xlg">
    <v-card-item class="pt-1">
      <v-row>

        <v-col cols="12" class="pb-1">
          <v-progress-linear :color="color" :model-value="progress" rounded height="6"></v-progress-linear>
        </v-col>

        <v-col cols="6">
          <p class="text-medium-emphasis">Used</p>
          <h4>{{ used }}</h4>
          <v-divider class="my-3"></v-divider>
          <p class="text-medium-emphasis">Percentage</p>
          <h4 class="">{{ percentage }}</h4>
        </v-col>

        <v-col cols="6">
          <p class="text-right text-medium-emphasis">Available</p>
          <h4 class="text-right">{{ available }}</h4>
          <v-divider class="my-3"></v-divider>
          <p class="text-right text-medium-emphasis">Limit</p>
          <h4 class="text-right">{{ limit }}</h4>
        </v-col>

        <v-divider></v-divider>

        <v-col>
          <v-btn size="small" variant="outlined" color="default" class="mt-1 my-2">Change Limits</v-btn>
        </v-col>

      </v-row>
    </v-card-item>
  </v-card>
</template>

<script>
export default {
  props: ['title', 'progress', 'used', 'limit', 'available', 'percentage', 'color']
}
</script>