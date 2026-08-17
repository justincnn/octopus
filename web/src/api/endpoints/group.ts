import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiRequest } from '../client';

/**
 * 分组项信息
 */
export interface GroupItem {
    id?: number;
    group_id?: number;
    channel_id: number;
    model_name: string;
    priority: number;
    weight: number;
    enabled?: boolean; // 启用才参与轮询; 加入≠启用
}

/**
 * 分组模式(兼容旧数据; 新 UI 用 item_strategy)
 */
export enum GroupMode {
    RoundRobin = 1,
    Random = 2,
    Failover = 3,
    Weighted = 4,
    Manual = 5,
}

/** 分组模型轮询策略(与渠道 key 池一致) */
export const ITEM_STRATEGIES = [
    { value: 'round_robin', label: '轮询' },
    { value: 'random', label: '随机' },
    { value: 'least_used', label: '最少使用' },
    { value: 'priority', label: '主模型优先' },
] as const;

/**
 * 分组信息
 */
export interface Group {
    id?: number;
    name: string;
    mode: GroupMode;
    item_strategy?: string;
    match_regex: string;
    first_token_time_out?: number;
    session_keep_time?: number;
    items?: GroupItem[];
}

/**
 * 新增 item 请求
 */
export interface GroupItemAddRequest {
    channel_id: number;
    model_name: string;
    priority: number;
    weight: number;
}

/**
 * 更新 item 请求 (priority/weight/enabled; 数组一次提交 = 批量)
 */
export interface GroupItemUpdateRequest {
    id: number;
    priority: number;
    weight: number;
    enabled?: boolean;
}

/**
 * 分组更新请求 - 仅包含变更的数据
 */
export interface GroupUpdateRequest {
    id: number;
    name?: string;                        // 仅在名称变更时发送
    mode?: GroupMode;                     // 仅在模式变更时发送
    item_strategy?: string;               // 仅在轮询策略变更时发送
    match_regex?: string;                 // 仅在匹配正则变更时发送
    first_token_time_out?: number;        // 仅在超时变更时发送
    session_keep_time?: number;           // 仅在会话保持时间变更时发送
    items_to_add?: GroupItemAddRequest[];    // 新增的 items
    items_to_update?: GroupItemUpdateRequest[]; // 更新的 items (priority/weight/enabled 变更)
    items_to_delete?: number[];              // 删除的 item IDs
}

/**
 * 获取分组列表 Hook
 * 
 * @example
 * const { data: groups, isLoading, error } = useGroupList();
 * 
 * if (isLoading) return <Loading />;
 * if (error) return <Error message={error.message} />;
 * 
 * groups?.forEach(group => console.log(group.name, group.items));
 */
export function useGroupList() {
    return useQuery({
        queryKey: ['groups', 'list'],
        queryFn: () => apiRequest<Group[]>('/api/v1/group/list'),
        refetchInterval: 30000,
        refetchOnMount: 'always',
    });
}

/**
 * 创建分组 Hook
 * 
 * @example
 * const createGroup = useCreateGroup();
 * 
 * createGroup.mutate({
 *   name: 'my-group',
 *   items: [
 *     { channel_id: 1, model_name: 'gpt-4', priority: 1 },
 *   ],
 * });
 */
export function useCreateGroup() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: (data: Group) =>
            apiRequest<Group>('/api/v1/group/create', { method: 'POST', body: data }),
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['groups', 'list'] }),
    });
}

/**
 * 更新分组 Hook - 仅发送变更的数据
 * 
 * @example
 * const updateGroup = useUpdateGroup();
 * 
 * updateGroup.mutate({
 *   id: 1,
 *   name: 'updated-group',  // 可选，仅在名称变更时发送
 *   items_to_add: [{ channel_id: 1, model_name: 'gpt-4', priority: 1 }],
 *   items_to_update: [{ id: 1, priority: 2 }],
 *   items_to_delete: [2, 3],
 * });
 */
export function useUpdateGroup() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: (data: GroupUpdateRequest) =>
            apiRequest<Group>('/api/v1/group/update', { method: 'POST', body: data }),
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['groups', 'list'] }),
    });
}

/**
 * 删除分组 Hook
 * 
 * @example
 * const deleteGroup = useDeleteGroup();
 * 
 * deleteGroup.mutate(1); // 删除 ID 为 1 的分组
 */
export function useDeleteGroup() {
    const queryClient = useQueryClient();

    return useMutation({
        mutationFn: (id: number) =>
            apiRequest<null>(`/api/v1/group/delete/${id}`, { method: 'DELETE' }),
        onSuccess: () => queryClient.invalidateQueries({ queryKey: ['groups', 'list'] }),
    });
}
