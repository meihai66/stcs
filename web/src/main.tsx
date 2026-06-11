import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import './index.css'

// 渲染前应用已保存的主题,避免浅色用户刷新时闪一下深色
if (localStorage.getItem('stcs-theme') === 'light') {
  document.documentElement.classList.add('light')
}

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
)
