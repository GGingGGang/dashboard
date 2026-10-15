export type JSONRequest = { source: "http"; url: string; filter: string };
export type JSONResult = { json: string; text: string; scalar: boolean; count: number; observedAt: string };

export function validateJSONRequest(value: unknown): JSONRequest {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("API 조회 설정이 올바르지 않습니다.");
  const request = value as Record<string, unknown>;
  if (Object.keys(request).length !== 3 || request.source !== "http" || typeof request.url !== "string" || typeof request.filter !== "string") throw new Error("현재 인증 없는 HTTP API 조회만 지원합니다.");
  const url = new URL(request.url);
  if (!["http:", "https:"].includes(url.protocol) || !url.hostname || url.username || url.password || url.hash || request.url.length > 8192) throw new Error("계정 정보가 없는 HTTP(S) URL을 입력하세요.");
  if (!request.filter.trim() || new TextEncoder().encode(request.filter).length > 16384) throw new Error("jq 식을 입력하세요. 최대 16KiB까지 지원합니다.");
  return { source: "http", url: request.url, filter: request.filter };
}
