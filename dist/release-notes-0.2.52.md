## v0.2.52

- 修复 v0.2.51 中部分 Client 详情页打开时报 React minified error #300 的问题。
- 原因是 Realm 配置复制下拉框引入了 React Hook，但托管配置面板此前以普通函数方式渲染；本版改为标准 React 组件渲染，避免 Hook 顺序在不同 Client/Tab 间变化。
