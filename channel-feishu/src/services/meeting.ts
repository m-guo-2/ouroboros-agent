import { getClient } from "../client";
import type { ReserveMeetingParams } from "../types";

const client = () => getClient();

// ==================== 视频会议 ====================

/**
 * 预约会议
 * https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/vc-v1/reserve/apply
 */
export async function reserveMeeting(params: ReserveMeetingParams) {
  const meetingSettings: any = {
    topic: params.topic,
    action_permissions: [
      {
        permission: 1, // 是否可共享屏幕
        permission_checkers: [{ check_field: 1, check_mode: 1, check_list: ["default"] }],
      },
    ],
    meeting_initial_type: 1, // 1: 多人会议
    call_setting: {
      callee: params.invitees?.map((inv) => ({
        id: inv.id,
        user_type: 1, // 飞书用户
      })) || [],
    },
  };

  if (params.settings?.password) {
    meetingSettings.meeting_connect_setting = {
      password: params.settings.password,
    };
  }

  const res = await client().vc.reserve.apply({
    params: {
      user_id_type: "open_id",
    },
    data: {
      end_time: params.endTime,
      meeting_settings: meetingSettings,
    },
  });

  return res;
}

/**
 * 获取会议详情
 * https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/vc-v1/meeting/get
 */
export async function getMeeting(meetingId: string) {
  const res = await client().vc.meeting.get({
    path: {
      meeting_id: meetingId,
    },
    params: {
      with_participants: true,
      user_id_type: "open_id",
    },
  });
  return res;
}

/**
 * 邀请参会人
 * https://open.feishu.cn/document/uAjLw4CM/ukTMukTMukTM/reference/vc-v1/meeting/invite
 */
export async function inviteToMeeting(
  meetingId: string,
  invitees: Array<{ id: string; userType?: number }>
) {
  const res = await client().vc.meeting.invite({
    path: {
      meeting_id: meetingId,
    },
    params: {
      user_id_type: "open_id",
    },
    data: {
      invitees: invitees.map((inv) => ({
        id: inv.id,
        user_type: inv.userType || 1,
      })),
    },
  });
  return res;
}

/**
 * 结束会议
 */
export async function endMeeting(meetingId: string) {
  const res = await client().vc.meeting.end({
    path: {
      meeting_id: meetingId,
    },
  });
  return res;
}

/**
 * 获取会议列表（通过日历事件方式查询）
 * 获取会议录制列表
 */
export async function getMeetingRecordingList(meetingId: string) {
  const res = await client().vc.meetingRecording.get({
    path: {
      meeting_id: meetingId,
    },
  });
  return res;
}

/**
 * 开始会议录制
 */
export async function startRecording(meetingId: string) {
  const res = await client().vc.meetingRecording.start({
    path: {
      meeting_id: meetingId,
    },
  });
  return res;
}

/**
 * 停止会议录制
 */
export async function stopRecording(meetingId: string) {
  const res = await client().vc.meetingRecording.stop({
    path: {
      meeting_id: meetingId,
    },
  });
  return res;
}
