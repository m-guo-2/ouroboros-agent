import { Router } from "express";
import * as meetingService from "../services/meeting";
import type { ApiResponse } from "../types";

const router = Router();

/** POST /api/feishu/meeting/reserve - 预约会议 */
router.post("/reserve", async (req, res) => {
  try {
    const { topic, startTime, endTime, invitees, settings } = req.body;
    if (!topic || !startTime || !endTime) {
      res.status(400).json({
        success: false,
        error: "topic, startTime, and endTime are required",
      } as ApiResponse);
      return;
    }

    const result = await meetingService.reserveMeeting({
      topic,
      startTime,
      endTime,
      invitees,
      settings,
    });
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("预约会议失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

/** GET /api/feishu/meeting/:meetingId - 获取会议详情 */
router.get("/:meetingId", async (req, res) => {
  try {
    const { meetingId } = req.params;
    const result = await meetingService.getMeeting(meetingId);
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("获取会议详情失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

/** POST /api/feishu/meeting/:meetingId/invite - 邀请参会人 */
router.post("/:meetingId/invite", async (req, res) => {
  try {
    const { meetingId } = req.params;
    const { invitees } = req.body;
    if (!invitees || !Array.isArray(invitees)) {
      res.status(400).json({ success: false, error: "invitees array is required" } as ApiResponse);
      return;
    }

    const result = await meetingService.inviteToMeeting(meetingId, invitees);
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("邀请参会人失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

/** POST /api/feishu/meeting/:meetingId/end - 结束会议 */
router.post("/:meetingId/end", async (req, res) => {
  try {
    const { meetingId } = req.params;
    const result = await meetingService.endMeeting(meetingId);
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("结束会议失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

/** GET /api/feishu/meeting/:meetingId/recording - 获取会议录制 */
router.get("/:meetingId/recording", async (req, res) => {
  try {
    const { meetingId } = req.params;
    const result = await meetingService.getMeetingRecordingList(meetingId);
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("获取会议录制失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

/** POST /api/feishu/meeting/:meetingId/recording/start - 开始录制 */
router.post("/:meetingId/recording/start", async (req, res) => {
  try {
    const { meetingId } = req.params;
    const result = await meetingService.startRecording(meetingId);
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("开始录制失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

/** POST /api/feishu/meeting/:meetingId/recording/stop - 停止录制 */
router.post("/:meetingId/recording/stop", async (req, res) => {
  try {
    const { meetingId } = req.params;
    const result = await meetingService.stopRecording(meetingId);
    res.json({ success: true, data: result } as ApiResponse);
  } catch (err: any) {
    console.error("停止录制失败:", err);
    res.status(500).json({ success: false, error: err.message } as ApiResponse);
  }
});

export default router;
