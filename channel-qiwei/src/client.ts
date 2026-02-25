/**
 * QiWe 开放平台 API 客户端
 *
 * 所有请求统一发往 POST {baseUrl}/api/qw/doApi
 * Header: X-QIWEI-TOKEN: {token}
 * Body: { method: "/path/to/api", params: { guid, ...extraParams } }
 */

import { qiweiConfig } from "./config";

interface DoApiRequest {
  method: string;
  params: Record<string, unknown>;
}

interface DoApiResponse {
  code: number;
  msg: string;
  data?: unknown;
}

/**
 * 调用 QiWe 统一 API
 */
export async function doApi(
  method: string,
  params: Record<string, unknown> = {}
): Promise<DoApiResponse> {
  const url = `${qiweiConfig.apiBaseUrl}/api/qw/doApi`;

  const body: DoApiRequest = {
    method,
    params: {
      guid: qiweiConfig.guid,
      ...params,
    },
  };

  try {
    const response = await fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-QIWEI-TOKEN": qiweiConfig.token,
      },
      body: JSON.stringify(body),
    });

    if (!response.ok) {
      const errText = await response.text();
      throw new Error(`QiWe API error: ${response.status} - ${errText}`);
    }

    const result = (await response.json()) as DoApiResponse;

    if (result.code !== 200 && result.code !== 0) {
      console.warn(`⚠️ QiWe API [${method}] returned code ${result.code}: ${result.msg}`);
    }

    return result;
  } catch (err) {
    const error = err as Error;
    console.error(`❌ QiWe API [${method}] failed:`, error.message);
    throw error;
  }
}
