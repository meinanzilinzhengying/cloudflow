import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import './styles/dark-theme.scss'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'
import VueECharts from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart, PieChart, BarChart, HeatmapChart, GraphChart } from 'echarts/charts'
import { GridComponent, TooltipComponent, LegendComponent, TitleComponent, DatasetComponent, VisualMapComponent, DataZoomComponent, GraphicComponent } from 'echarts/components'

use([CanvasRenderer, LineChart, PieChart, BarChart, HeatmapChart, GraphChart, GridComponent, TooltipComponent, LegendComponent, TitleComponent, DatasetComponent, VisualMapComponent, DataZoomComponent, GraphicComponent])

import App from './App.vue'
import router from './router'
import { createPinia } from 'pinia'

const app = createApp(App)

app.component('v-chart', VueECharts)

for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.use(ElementPlus)
app.use(createPinia())
app.use(router)
app.mount('#app')
