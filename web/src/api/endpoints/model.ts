import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiRequest } from '../client';

/**
 * LLM 价格信息
 */
export interface LLMPrice {
    input: number;
    output: number;
    cache_read: number;
    cache_write: number;
}

/**
 * LLM 模型信息
 */
export interface LLMInfo extends LLMPrice {
    name: string;
}

/**
 * LLM 渠道关联信息
 */
export interface LLMChannel {
    name: string;
    enabled: boolean;
    channel_id: number;
    channel_name: string;
}

/**
 * 获取 LLM 模型列表 Hook
 * 
 * @example
 * const { data: models, isLoading, error } = useModelList();
 * 
 * if (isLoading) return <Loading />;
 * if (error) return <Error message={error.message} />;
 * 
 * models?.forEach(model => console.log(model.name, model.input));
 */
export function useModelList() {
    return useQuery({
        queryKey: ['models', 'list'],
        queryFn: () => apiRequest<LLMInfo[]>('/api/v1/model/list'),
        refetchInterval: 30000,
        refetchOnMount: 'always',
    });
}

/**
 * 获取 LLM 模型与渠道关联列表 Hook
 * 
 * @example
 * const { data: channelModels, isLoading, error } = useModelChannelList();
 * 
 * if (isLoading) return <Loading />;
 * if (error) return <Error message={error.message} />;
 * 
 * channelModels?.forEach(item => console.log(item.name, item.channel_name));
 */
export function useModelChannelList() {
    return useQuery({
        queryKey: ['models', 'channel'],
        queryFn: () => apiRequest<LLMChannel[]>('/api/v1/model/channel'),
        refetchInterval: 30000,
    });
}

/**
 * 更新 LLM 模型 Hook
 * 
 * @example
 * const updateModel = useUpdateModel();
 * 
 * updateModel.mutate({
 *   name: 'gpt-4',
 *   input: 0.03,
 *   output: 0.06,
 *   cache_read: 0.015,
 *   cache_write: 0.03,
 * });
 */
export function useUpdateModel() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: (data: LLMInfo) =>
            apiRequest<LLMInfo>('/api/v1/model/update', { method: 'POST', body: data }),
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['models', 'list'] }),
    });
}

/**
 * 创建 LLM 模型 Hook
 * 
 * @example
 * const createModel = useCreateModel();
 * 
 * createModel.mutate({
 *   name: 'gpt-4',
 *   input: 0.03,
 *   output: 0.06,
 *   cache_read: 0.015,
 *   cache_write: 0.03,
 * });
 */
export function useCreateModel() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: (data: LLMInfo) =>
            apiRequest<LLMInfo>('/api/v1/model/create', { method: 'POST', body: data }),
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['models', 'list'] }),
    });
}

/**
 * 删除 LLM 模型 Hook
 * 
 * @example
 * const deleteModel = useDeleteModel();
 * 
 * deleteModel.mutate('gpt-4'); // 删除名称为 'gpt-4' 的模型
 */
export function useDeleteModel() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: (name: string) =>
            apiRequest<null>('/api/v1/model/delete', { method: 'POST', body: { name } }),
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['models', 'list'] }),
    });
}

/**
 * 更新 LLM 模型价格 Hook
 * 
 * @example
 * const updatePrice = useUpdateModelPrice();
 * 
 * updatePrice.mutate(); // 触发价格更新
 */
export function useUpdateModelPrice() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: () => apiRequest<null>('/api/v1/model/update-price', { method: 'POST', body: {} }),
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['models', 'last-update-time'] }),
    });
}

/**
 * 获取 LLM 模型价格最后更新时间 Hook
 * 
 * @example
 * const { data: lastUpdateTime } = useLastUpdateTime();
 * 
 * if (lastUpdateTime) {
 *   console.log('最后更新:', new Date(lastUpdateTime).toLocaleString());
 * }
 */
export function useLastUpdateTime() {
    return useQuery({
        queryKey: ['models', 'last-update-time'],
        queryFn: () => apiRequest<string>('/api/v1/model/last-update-time'),
        refetchInterval: 30000,
    });
}

// ---- 未匹配模型模糊匹配 ----

export interface ModelMatchCandidate {
    canonical_id: string;
    reason: string;
    price: {
        input: number;
        output: number;
        cache_read: number;
        cache_write: number;
    };
}

/** 批量匹配结果: 模型名 + 候选列表 */
export interface BatchMatchResult {
    name: string;
    candidates: ModelMatchCandidate[];
}

/** 获取所有渠道里未匹配价格的模型名 */
export function useUnmatchedModels(enabled = true) {
    return useQuery({
        queryKey: ['models', 'unmatched'],
        queryFn: () => apiRequest<string[]>('/api/v1/model/unmatched'),
        refetchInterval: 30000,
        enabled,
    });
}

/** 获取渠道已选但没进任何分组的模型(含来源渠道) */
export interface UngroupedModel {
    name: string;
    channel_id: number;
    channel_name: string;
}

/** 获取渠道已选但没进任何分组的模型(分组页"未分组 N"徽章) */
export function useUngroupedModels(enabled = true) {
    return useQuery({
        queryKey: ['models', 'ungrouped'],
        queryFn: () => apiRequest<UngroupedModel[]>('/api/v1/model/ungrouped'),
        refetchInterval: 30000,
        enabled,
    });
}

/** 对某个模型名做模糊匹配, 返回候选 */
export function useMatchModel() {
    return useMutation({
        mutationFn: (name: string) =>
            apiRequest<ModelMatchCandidate[]>('/api/v1/model/match', { method: 'POST', body: { name } }),
    });
}

/** 一键批量匹配: 对全部未匹配模型返回只读候选, 前端逐条确认后才写别名 */
export function useMatchAll() {
    return useMutation({
        mutationFn: () => apiRequest<BatchMatchResult[]>('/api/v1/model/match-all'),
    });
}

/** 确认一条别名映射(渠道模型名 → models.dev 规范名), 立即写价格 */
export function useSetModelAlias() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: (data: { src: string; canonical: string }) =>
            apiRequest<null>('/api/v1/model/alias', { method: 'POST', body: data }),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['models', 'unmatched'] });
            queryClient.invalidateQueries({ queryKey: ['models', 'list'] });
            queryClient.invalidateQueries({ queryKey: ['models', 'aliases'] });
        },
    });
}

/** 全部已匹配别名映射 src → canonical */
export function useModelAliases() {
    return useQuery({
        queryKey: ['models', 'aliases'],
        queryFn: () => apiRequest<Record<string, string>>('/api/v1/model/alias'),
    });
}

/** 全量价格目录关键词搜索(只读, 至少 2 字符才触发) */
export function useSearchModels(q: string) {
    return useQuery({
        queryKey: ['models', 'search', q],
        queryFn: () =>
            apiRequest<ModelMatchCandidate[]>(`/api/v1/model/search?q=${encodeURIComponent(q)}`),
        enabled: q.trim().length >= 2,
    });
}

/** 删除一条别名映射(同时清价格, 模型回到未匹配池) */
export function useDeleteModelAlias() {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: (src: string) =>
            apiRequest<null>('/api/v1/model/alias', { method: 'DELETE', body: { src } }),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['models', 'unmatched'] });
            queryClient.invalidateQueries({ queryKey: ['models', 'list'] });
            queryClient.invalidateQueries({ queryKey: ['models', 'aliases'] });
        },
    });
}
