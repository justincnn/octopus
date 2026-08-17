import { useMemo, useState } from 'react';
import { KeyRound, Plus, Trash2, RotateCcw, Ban, CheckCircle2, XCircle, Loader2, AlertTriangle } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { toast } from '@/components/common/Toast';
import {
    type ChannelKeyStatus,
    useChannelKeysStatus,
    useDisableChannelKey,
    useRecoverChannelKey,
} from '@/api/endpoints/channel';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';

// 轮询策略选项
const STRATEGIES: { value: string; label: string; desc: string }[] = [
    { value: 'priority', label: '主 key 优先', desc: '当前 key 正常就一直用它，失效后随机换另一个' },
    { value: 'round_robin', label: '顺序轮询', desc: '按顺序轮流使用每个 key' },
    { value: 'random', label: '随机', desc: '每次从可用 key 中随机选' },
    { value: 'least_used', label: '最少使用', desc: '优先用累计使用次数最少的 key' },
];

// 原因 → 中文文案 + 颜色
const REASON_META: Record<string, { label: string; color: string }> = {
    '': { label: '正常', color: 'text-emerald-500' },
    invalid: { label: '认证失败 (401/403)', color: 'text-red-500' },
    rate_limited: { label: '限流 (429)', color: 'text-amber-500' },
    upstream_error: { label: '上游错误 (5xx/网络)', color: 'text-orange-500' },
    disabled: { label: '手动禁用', color: 'text-slate-400' },
};

function maskKey(key: string) {
    if (key.length <= 8) return '****';
    return key.slice(0, 3) + '****' + key.slice(-4);
}

function fmtTime(unixSec: number) {
    if (!unixSec) return '';
    const diff = Math.floor(Date.now() / 1000) - unixSec;
    if (diff < 60) return `${diff}s前`;
    if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`;
    if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`;
    return new Date(unixSec * 1000).toLocaleDateString();
}

export function KeysManagerDialog({
    channelId,
    initialKeys,
    strategy,
    onSave,
    onStrategyChange,
    onClose,
}: {
    channelId: number;
    initialKeys: string[];
    strategy: string;
    onSave: (keys: string[]) => void;
    onStrategyChange: (strategy: string) => void;
    onClose: () => void;
}) {
    const [editing, setEditing] = useState(false);
    const [draft, setDraft] = useState(initialKeys.join('\n'));
    const [saving, setSaving] = useState(false);
    const { data: statusList, refetch } = useChannelKeysStatus(channelId, true);
    const disableKey = useDisableChannelKey();
    const recoverKey = useRecoverChannelKey();

    // 支持换行/空格/逗号分隔批量粘贴(用户可能整段粘贴)
    const draftKeys = useMemo(
        () =>
            draft
                .split(/[\n\s,]+/)
                .map((s) => s.trim())
                .filter(Boolean),
        [draft]
    );

    const handleSave = () => {
        setSaving(true);
        onSave(draftKeys);
        setSaving(false);
        setEditing(false);
    };

    const statusMap = useMemo(() => {
        const m = new Map<string, ChannelKeyStatus>();
        (statusList ?? []).forEach((s) => m.set(s.key, s));
        return m;
    }, [statusList]);

    const handleDisable = (key: string, disabled: boolean) => {
        disableKey.mutate(
            { channel_id: channelId, key, disabled },
            {
                onSuccess: () => {
                    toast.success(disabled ? '已禁用' : '已启用');
                    void refetch();
                },
                onError: () => toast.error('操作失败'),
            }
        );
    };

    const handleRecover = (key: string) => {
        recoverKey.mutate(
            { channel_id: channelId, key },
            {
                onSuccess: () => {
                    toast.success('已恢复');
                    void refetch();
                },
                onError: () => toast.error('恢复失败'),
            }
        );
    };

    const pendingOps = disableKey.isPending || recoverKey.isPending;

    return (
        <div className="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 p-4">
            <div className="w-full max-w-lg rounded-2xl border border-border bg-card p-5 shadow-2xl">
                <h3 className="mb-1 flex items-center gap-2 text-base font-bold text-card-foreground">
                    <KeyRound className="h-4 w-4" />Keys 管理
                </h3>
                <p className="mb-4 text-xs text-muted-foreground">
                    {initialKeys.length} 个 key · 轮询使用 · 无效 key 自动跳过，每小时自动探测恢复
                </p>

                {/* 轮询策略选择 */}
                <div className="mb-4 space-y-1.5">
                    <label className="text-xs font-medium text-card-foreground">轮询策略</label>
                    <Select value={strategy || 'priority'} onValueChange={onStrategyChange}>
                        <SelectTrigger className="w-full rounded-xl">
                            <SelectValue placeholder="选择轮询策略" />
                        </SelectTrigger>
                        <SelectContent>
                            {STRATEGIES.map((s) => (
                                <SelectItem key={s.value} value={s.value}>
                                    {s.label}
                                    <span className="ml-2 text-xs text-muted-foreground">{s.desc}</span>
                                </SelectItem>
                            ))}
                        </SelectContent>
                    </Select>
                </div>

                {/* 编辑模式: 多行输入 */}
                {editing ? (
                    <div className="space-y-3">
                        <textarea
                            value={draft}
                            onChange={(e) => setDraft(e.target.value)}
                            placeholder={'每行一个 key，可直接粘贴多个'}
                            rows={6}
                            className="w-full rounded-xl border border-border bg-muted/30 p-3 font-mono text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
                        />
                        <div className="flex justify-end gap-2">
                            <Button type="button" variant="ghost" size="sm" onClick={() => setEditing(false)} className="rounded-xl">
                                取消
                            </Button>
                            <Button type="button" size="sm" onClick={handleSave} disabled={saving} className="rounded-xl">
                                {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin mr-1" /> : <CheckCircle2 className="h-3.5 w-3.5 mr-1" />}
                                保存 ({draftKeys.length})
                            </Button>
                        </div>
                    </div>
                ) : (
                    <div className="space-y-3">
                        {/* 状态列表 */}
                        <div className="max-h-80 overflow-y-auto space-y-2 rounded-xl border border-border bg-muted/30 p-2">
                            {initialKeys.length === 0 && (
                                <div className="py-8 text-center text-xs text-muted-foreground">
                                    暂无 key，点击「编辑 Keys」添加
                                </div>
                            )}
                            {initialKeys.map((key) => {
                                const st = statusMap.get(key);
                                const active = st ? st.status === 'active' && !st.disabled : true;
                                const meta = REASON_META[st?.reason ?? ''] ?? REASON_META[''];
                                return (
                                    <div
                                        key={key}
                                        className="flex items-center justify-between gap-2 rounded-lg border border-border/60 bg-card px-3 py-2"
                                    >
                                        <div className="min-w-0">
                                            <div className="flex items-center gap-2">
                                                <code className="truncate font-mono text-xs text-foreground">{maskKey(key)}</code>
                                                {active ? (
                                                    <span className="flex items-center gap-0.5 text-[10px] text-emerald-500">
                                                        <CheckCircle2 className="h-3 w-3" />正常
                                                    </span>
                                                ) : (
                                                    <span className={`flex items-center gap-0.5 text-[10px] ${meta.color}`}>
                                                        <XCircle className="h-3 w-3" />{meta.label}
                                                    </span>
                                                )}
                                            </div>
                                            {!active && (
                                                <div className="mt-0.5 flex items-center gap-2 text-[10px] text-muted-foreground">
                                                    {st?.fail_count ? <span>连续失败 {st.fail_count} 次</span> : null}
                                                    {st?.last_fail_at ? <span>· {fmtTime(st.last_fail_at)}</span> : null}
                                                </div>
                                            )}
                                        </div>
                                        <div className="flex shrink-0 items-center gap-1">
                                            {st?.disabled ? (
                                                <Button type="button" variant="outline" size="sm" className="h-7 rounded-lg px-2 text-xs" disabled={pendingOps} onClick={() => handleDisable(key, false)}>
                                                    <CheckCircle2 className="h-3 w-3 mr-1" />启用
                                                </Button>
                                            ) : st?.status === 'invalid' ? (
                                                <>
                                                    <Button type="button" variant="outline" size="sm" className="h-7 rounded-lg px-2 text-xs" disabled={pendingOps} onClick={() => handleRecover(key)}>
                                                        <RotateCcw className="h-3 w-3 mr-1" />恢复
                                                    </Button>
                                                    <Button type="button" variant="ghost" size="sm" className="h-7 rounded-lg px-2 text-xs text-slate-400" disabled={pendingOps} onClick={() => handleDisable(key, true)}>
                                                        <Ban className="h-3 w-3 mr-1" />禁用
                                                    </Button>
                                                </>
                                            ) : (
                                                <Button type="button" variant="ghost" size="sm" className="h-7 rounded-lg px-2 text-xs text-slate-400" disabled={pendingOps} onClick={() => handleDisable(key, true)}>
                                                    <Ban className="h-3 w-3 mr-1" />禁用
                                                </Button>
                                            )}
                                        </div>
                                    </div>
                                );
                            })}
                        </div>

                        {/* 提示 */}
                        <div className="flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/5 p-2.5 text-[11px] text-amber-600">
                            <AlertTriangle className="h-3.5 w-3.5 mt-0.5 shrink-0" />
                            <span>
                                多个 key 将按轮询顺序使用；401/403 连错 3 次自动失效并跳过，429 连错 3 次标记限流。失效 key 每小时自动探测恢复（间隔 10~30s）。
                            </span>
                        </div>

                        <div className="flex justify-end gap-2">
                            <Button type="button" variant="ghost" size="sm" onClick={onClose} className="rounded-xl">
                                关闭
                            </Button>
                            <Button type="button" size="sm" onClick={() => setEditing(true)} className="rounded-xl">
                                <Plus className="h-3.5 w-3.5 mr-1" />编辑 Keys
                            </Button>
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
}
