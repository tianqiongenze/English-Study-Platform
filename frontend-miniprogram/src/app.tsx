import { Component } from "react";
import "./app.scss";

// 全局:从缓存恢复 token;登录态存 wx.storage(小程序无 localStorage)。
export interface AuthUser { id: number; email: string; name: string; role: string }

class App extends Component {
  componentDidMount() {
    const token = wx.getStorageSync("esp_token");
    if (token) (this as any).token = token;
  }
  render() { return (this as any).props.children; }
}

export default App;
