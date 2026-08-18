import { useMemo, useState } from 'react';
import { Loader2, Plus, Wand2, XCircle } from 'lucide-react';
import { useTranslations } from 'use-intl';
import { useGroupList, useUpdateGroup } from '@/api/endpoints/group';
import { useUngroupedModels } from '@/api/endpoints/model';
import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { toast } from 'sonner';

/**
 * 未分组模型管理弹窗: 列出渠道已选但没进任何分组的模型,
 * 支持单行「加入分组」(下拉选目标分组)与「自动匹配」(按分组 match_regex 批量加入)。
 */
export function UngroupedModelsDialog() {
    const t = useTranslations('model');
    const { data: ungrouped, refetch } = useUngroupedModels(true);
    const { data: groups } = useGroupList();
    const updateGroup = useUpdateGroup();

    // 每行的目标分组选择: model_name -> group_id
    const [targets, setTargets] = useState<Record<string, number>>({});
    // 加入中的模型(防止重复点击)
    const [adding, setAdding] = useState<Record<string, boolean>>({});

    const list = ungrouped ?? [];
    const groupOptions = useMemo(() => (groups ?? []).filter((g) => g.id != null), [groups]);

    const doAdd = (name: string, channelId: number, groupId: number) => {
        setAdding((p) => ({ ...p, [name]: true }));
        updateGroup.mutate(
            { id: groupId, items_to_add: [{ channel_id: channelId, model_name: name, priority: 0, weight: 1 }] },
            {
                onSuccess: () => {
                    toast.success(`已加入分组: ${name}`);
                    void refetch();
                },
                onError: () => toast.error(`加入失败: ${name}`),
                onSettled: () => setAdding((p) => ({ ...p, [name]: false })),
            }
        );
    };

    // 自动匹配: 按分组 match_regex 匹配模型名, 批量加入第一个命中分组
    const handleAutoMatch = () => {
        let matched = 0;
        list.forEach((m) => {
            const group = groupOptions.find((g) => {
                if (!g.match_regex) return false;
                try {
                    return new RegExp(g.match_regex, 'i').test(m.name);
                } catch {
                    return false;
                }
            });
            if (group?.id != null) {
                doAdd(m.name, m.channel_id, group.id);
                matched++;
            }
        });
        if (matched === 0) toast.info('没有可自动匹配的模型(无命中 match_regex)');
    };

    return (
        <div className="w-[440px]">
            <div className="mb-3 flex items-center justify-between">
                <h3 className="text-base font-bold text-card-foreground">未分组模型 ({list.length})</h3>
                <Button type="button" variant="outline" size="sm" className="h-7 rounded-lg px-2 text-xs" onClick={handleAutoMatch} disabled={list.length === 0}>
                    <Wand2 className="h-3 w-3 mr-1" />自动匹配
                </Button>
            </div>
            <p className="mb-3 text-xs text-muted-foreground">
                这些模型已在渠道启用但未进任何分组，加入分组后才能路由转发
            </p>

            <div className="max-h-80 space-y-1.5 overflow-y-auto rounded-xl border border-border bg-muted/30 p-2">
                {list.length === 0 && (
                    <div className="py-8 text-center text-xs text-muted-foreground">全部模型已分组 🎉</div>
                )}
                {list.map((m) => (
                    <div key={`${m.channel_id}-${m.name}`} className="flex items-center gap-2 rounded-lg border border-border/60 bg-card px-2.5 py-1.5">
                        <div className="min-w-0 flex-1">
                            <div className="truncate font-mono text-xs text-foreground">{m.name}</div>
                            <div className="truncate text-[10px] text-muted-foreground">{m.channel_name}</div>
                        </div>
                        <Select
                            value={targets[m.name] != null ? String(targets[m.name]) : undefined}
                            onValueChange={(v) => setTargets((p) => ({ ...p, [m.name]: Number(v) }))}
                        >
                            <SelectTrigger className="h-7 w-28 rounded-lg text-xs">
                                <SelectValue placeholder="选分组" />
                            </SelectTrigger>
                            <SelectContent>
                                {groupOptions.map((g) => (
                                    <SelectItem key={g.id} value={String(g.id)}>
                                        {g.name}
                                    </SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                        <Button
                            type="button"
                            size="sm"
                            className="h-7 rounded-lg px-2 text-xs"
                            disabled={targets[m.name] == null || adding[m.name]}
                            onClick={() => doAdd(m.name, m.channel_id, targets[m.name])}
                        >
                            {adding[m.name] ? <Loader2 className="h-3 w-3 animate-spin" /> : <Plus className="h-3 w-3" />}
                            加入
                        </Button>
                    </div>
                ))}
            </div>
        </div>
    );
}
