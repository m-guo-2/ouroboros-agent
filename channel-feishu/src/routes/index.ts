import { Router } from "express";
import actionRoutes from "./action";
import messageRoutes from "./message";
import meetingRoutes from "./meeting";
import documentRoutes from "./document";
import sendRoutes from "./send";

const router = Router();

// ==================== 统一 Action 端点（Agent 调用入口） ====================
router.use("/action", actionRoutes);

// ==================== 渠道回调端点（server → channel-feishu 发送消息） ====================
router.use("/send", sendRoutes);

// ==================== 分模块 REST API ====================
// 消息相关 API
router.use("/message", messageRoutes);

// 会议相关 API
router.use("/meeting", meetingRoutes);

// 文档相关 API
router.use("/document", documentRoutes);

export default router;
