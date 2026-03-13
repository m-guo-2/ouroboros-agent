package runner

import (
	"context"
	"fmt"
	"time"

	"agent/internal/logger"
	"agent/internal/storage"
)

const schedulerInterval = 30 * time.Second

func StartDelayedTaskScheduler(ctx context.Context) {
	logger.Boundary(ctx, "延时任务调度器已启动", "interval", schedulerInterval.String())

	ticker := time.NewTicker(schedulerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Boundary(ctx, "延时任务调度器已停止")
			return
		case <-ticker.C:
			dispatchDueTasks(ctx)
		}
	}
}

func dispatchDueTasks(ctx context.Context) {
	tasks, err := storage.QueryDueTasks()
	if err != nil {
		logger.Error(ctx, "扫描到期任务失败", "error", err.Error())
		return
	}
	if len(tasks) == 0 {
		return
	}

	logger.Business(ctx, "发现到期任务", "count", len(tasks))

	for _, task := range tasks {
		if err := storage.MarkTaskDispatched(task.ID); err != nil {
			logger.Warn(ctx, "标记任务已投递失败", "taskId", task.ID, "error", err.Error())
			continue
		}

		content := formatDelayedTaskEvent(task)
		msgID := fmt.Sprintf("delayed-task-%s", task.ID)

		err := EnqueueProcessRequest(ctx, ProcessRequest{
			UserID:                task.UserID,
			AgentID:               task.AgentID,
			Content:               content,
			Channel:               task.Channel,
			ChannelUserID:         task.ChannelUserID,
			ChannelConversationID: task.ChannelConversationID,
			MessageType:           "text",
			MessageID:             msgID,
			SessionID:             task.SessionID,
		})
		if err != nil {
			logger.Error(ctx, "投递延时任务失败", "taskId", task.ID, "error", err.Error())
			continue
		}

		logger.Business(ctx, "延时任务已投递",
			"taskId", task.ID, "sessionId", task.SessionID, "agentId", task.AgentID)
	}
}

func formatDelayedTaskEvent(task storage.DelayedTask) string {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	return fmt.Sprintf(`【系统事件：定时任务到期】
task_id: %s
创建时间: %s
计划执行时间: %s
实际触发时间: %s
任务内容：%s`,
		task.ID, task.CreatedAt, task.ExecuteAt, now, task.Task)
}
