import { useMemo, useState } from 'react';
import { Pencil, Trash2, X, Power } from 'lucide-react';
import { motion, AnimatePresence } from 'motion/react';
import { useTranslations } from 'use-intl';
import { toast } from 'sonner';
import {
    type Group,
    useDeleteGroup,
    useUpdateGroup,
    ITEM_STRATEGIES,
} from '@/api/endpoints/group';
import { useModelChannelList } from '@/api/endpoints/model';
import { buildChannelNameByModelKey } from './utils';
import { buildDisplayMembers, buildGroupUpdatePayload, EditDialogContent } from './Card';
import {
    MorphingDialog,
    MorphingDialogContainer,
    MorphingDialogContent,
    MorphingDialogTrigger,
} from '@/components/ui/morphing-dialog';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/animate-ui/components/animate/tooltip';
import type { GroupEditorValues } from './Editor';

function StrategyBadge({ strategy }: { strategy?: string }) {
    const label = ITEM_STRATEGIES.find((s) => s.value === (strategy || 'round_robin'))?.label ?? strategy ?? '轮询';
    return (
        <span className="shrink-0 rounded-md bg-muted px-1.5 py-0.5 text-[11px] text-muted-foreground">
            {label}
        </span>
    );
}

/** 分组列表行: 紧凑概览(分组名/候选数/启用数/策略 + 编辑/删除)。 */
export function GroupListRow({ group }: { group: Group }) {
    const t = useTranslations('group');
    const updateGroup = useUpdateGroup();
    const deleteGroup = useDeleteGroup();
    const { data: modelChannels = [] } = useModelChannelList();
    const [confirmDelete, setConfirmDelete] = useState(false);

    const channelNameByKey = useMemo(() => buildChannelNameByModelKey(modelChannels), [modelChannels]);
    const displayMembers = useMemo(
        () => buildDisplayMembers(group, channelNameByKey),
        [group, channelNameByKey]
    );

    const items = group.items ?? [];
    const candidateCount = items.length;
    const enabledCount = items.filter((it) => it.enabled !== false).length;

    const onSuccess = () => toast.success(t('toast.updated'));
    const onError = (error: Error) => toast.error(t('toast.updateFailed'), { description: error.message });

    const handleSubmitEdit = (values: GroupEditorValues, onDone?: () => void) => {
        if (!group.id) return;
        const payload = buildGroupUpdatePayload(group, values);
        if (Object.keys(payload).length === 1) {
            onDone?.();
            return;
        }
        updateGroup.mutate(payload, {
            onSuccess: () => {
                onSuccess();
                onDone?.();
            },
            onError,
        });
    };

    return (
        <div className="relative flex items-center gap-3 rounded-xl border border-border bg-card px-3 py-2.5 text-card-foreground overflow-visible">
            <Tooltip side="top" sideOffset={8}>
                <TooltipTrigger asChild>
                    <span className="min-w-0 flex-1 truncate text-sm font-semibold">{group.name}</span>
                </TooltipTrigger>
                <TooltipContent>{group.name}</TooltipContent>
            </Tooltip>

            <span className="shrink-0 flex items-center gap-1 text-xs text-muted-foreground" title="候选模型数">
                <span className="rounded-md bg-muted px-1.5 py-0.5 text-[11px]">
                    候选 <b className="text-foreground">{candidateCount}</b>
                </span>
            </span>
            <span className="shrink-0 flex items-center gap-1 text-xs" title="启用模型数">
                <span className={enabledCount > 0 ? 'rounded-md bg-primary/10 px-1.5 py-0.5 text-[11px] text-primary' : 'rounded-md bg-muted px-1.5 py-0.5 text-[11px] text-muted-foreground'}>
                    <Power className="inline size-3 -mt-0.5 mr-0.5" />
                    启用 <b>{enabledCount}</b>
                </span>
            </span>
            <StrategyBadge strategy={group.item_strategy} />

            <div className="flex items-center gap-0.5 shrink-0">
                <MorphingDialog>
                    <MorphingDialogTrigger className="p-1.5 rounded-lg transition-colors hover:bg-muted text-muted-foreground hover:text-foreground">
                        <Tooltip side="top" sideOffset={8}>
                            <TooltipTrigger asChild>
                                <Pencil className="size-4" />
                            </TooltipTrigger>
                            <TooltipContent>{t('detail.actions.edit')}</TooltipContent>
                        </Tooltip>
                    </MorphingDialogTrigger>
                    <MorphingDialogContainer>
                        <MorphingDialogContent className="relative w-screen max-w-full md:max-w-4xl bg-card text-card-foreground px-6 py-4 rounded-3xl h-[calc(100vh-2rem)] flex flex-col overflow-hidden">
                            <EditDialogContent
                                group={group}
                                displayMembers={displayMembers}
                                isSubmitting={updateGroup.isPending}
                                onSubmit={handleSubmitEdit}
                            />
                        </MorphingDialogContent>
                    </MorphingDialogContainer>
                </MorphingDialog>

                {!confirmDelete && (
                    <Tooltip side="top" sideOffset={8}>
                        <TooltipTrigger>
                            <motion.button
                                layoutId={`list-delete-btn-group-${group.id}`}
                                type="button"
                                onClick={() => setConfirmDelete(true)}
                                className="p-1.5 rounded-lg hover:bg-destructive/10 text-muted-foreground hover:text-destructive transition-colors"
                            >
                                <Trash2 className="size-4" />
                            </motion.button>
                        </TooltipTrigger>
                        <TooltipContent>{t('detail.actions.delete')}</TooltipContent>
                    </Tooltip>
                )}
            </div>

            <AnimatePresence>
                {confirmDelete && (
                    <motion.div
                        layoutId={`list-delete-btn-group-${group.id}`}
                        className="absolute inset-0 flex items-center justify-center gap-2 bg-destructive p-2 rounded-xl"
                        transition={{ type: 'spring', stiffness: 400, damping: 30 }}
                    >
                        <button
                            type="button"
                            onClick={() => setConfirmDelete(false)}
                            className="flex h-7 w-7 items-center justify-center rounded-lg bg-destructive-foreground/20 text-destructive-foreground transition-all hover:bg-destructive-foreground/30 active:scale-95"
                        >
                            <X className="size-4" />
                        </button>
                        <button
                            type="button"
                            onClick={() => group.id && deleteGroup.mutate(group.id, { onSuccess: () => toast.success(t('toast.deleted')) })}
                            disabled={deleteGroup.isPending}
                            className="flex-1 h-7 flex items-center justify-center gap-2 rounded-lg bg-destructive-foreground text-destructive text-sm font-semibold transition-all hover:bg-destructive-foreground/90 active:scale-[0.98] disabled:opacity-50 disabled:cursor-not-allowed"
                        >
                            <Trash2 className="size-3.5" />
                            {t('detail.actions.confirmDelete')}
                        </button>
                    </motion.div>
                )}
            </AnimatePresence>
        </div>
    );
}
