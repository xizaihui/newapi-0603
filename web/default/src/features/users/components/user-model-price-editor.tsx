/*
专属模型按次价格编辑器 (Phase 3)

为该用户单独配置某些模型的按次价格（仅对按次计费的模型有效）。
- 模型从全局按次价格表中选择
- price 必须 >= 0；新价 > 全局价 10 倍时弹确认避免误填
- 优先级最高：覆盖全局按次价；最终扣费仍会与「分组倍率」相乘
*/
import { useEffect, useMemo, useState } from 'react'
import { toast } from 'sonner'
import { Trash2, Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  getUserModelPrices,
  setUserModelPrices,
  getPricing,
  type UserModelPrice,
  type PricingItem,
} from '../api'

type Props = {
  userId: number
}

type Row = {
  model_name: string
  price: string // 保留字符串以便用户中间态输入
  note: string
}

export function UserModelPriceEditor({ userId }: Props) {
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [rows, setRows] = useState<Row[]>([])
  const [allModels, setAllModels] = useState<PricingItem[]>([])
  const [addingModel, setAddingModel] = useState<string>('')

  // 拉取数据
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    Promise.all([getUserModelPrices(userId), getPricing()])
      .then(([overridesRes, pricingRes]) => {
        if (cancelled) return
        const overrides = (overridesRes.data || []) as UserModelPrice[]
        setRows(
          overrides.map((o) => ({
            model_name: o.model_name,
            price: String(o.price),
            note: o.note ?? '',
          }))
        )
        const items = (pricingRes.data || []) as PricingItem[]
        // 仅展示有 model_price 的模型（按次计费），与本功能语义一致
        setAllModels(items.filter((i) => (i.model_price ?? 0) > 0))
      })
      .catch(() => {
        // 静默：非管理员调不到
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [userId])

  const modelPriceMap = useMemo(() => {
    const m: Record<string, number> = {}
    for (const it of allModels) {
      if (it.model_price !== undefined) m[it.model_name] = it.model_price
    }
    return m
  }, [allModels])

  const usedModels = useMemo(
    () => new Set(rows.map((r) => r.model_name)),
    [rows]
  )

  const handleAdd = () => {
    if (!addingModel) {
      toast.error('请先选择一个模型')
      return
    }
    if (usedModels.has(addingModel)) {
      toast.error('该模型已在覆盖列表中')
      return
    }
    const globalPrice = modelPriceMap[addingModel] ?? 0
    setRows((prev) => [
      ...prev,
      {
        model_name: addingModel,
        price: String(globalPrice), // 默认填全局价
        note: '',
      },
    ])
    setAddingModel('')
  }

  const handleChangeRow = (idx: number, key: keyof Row, value: string) => {
    setRows((prev) => prev.map((r, i) => (i === idx ? { ...r, [key]: value } : r)))
  }

  const handleRemoveRow = (idx: number) => {
    setRows((prev) => prev.filter((_, i) => i !== idx))
  }

  const handleSave = async () => {
    // 校验
    const payload: Array<{ model_name: string; price: number; note?: string }> = []
    for (const r of rows) {
      const trimmed = r.price.trim()
      if (trimmed === '') {
        toast.error(`模型 "${r.model_name}" 的价格不能为空`)
        return
      }
      const n = Number(trimmed)
      if (!isFinite(n) || n < 0) {
        toast.error(`模型 "${r.model_name}" 的价格无效：${trimmed}`)
        return
      }
      // 限制最多 4 位小数，避免运营误填超长小数
      const dotIdx = trimmed.indexOf('.')
      if (dotIdx >= 0 && trimmed.length - dotIdx - 1 > 4) {
        toast.error(`模型 "${r.model_name}" 的价格最多支持 4 位小数：${trimmed}`)
        return
      }
      // 防误操作：价格 > 全局价 * 10 弹确认
      const globalPrice = modelPriceMap[r.model_name] ?? 0
      if (globalPrice > 0 && n > globalPrice * 10) {
        const ok = window.confirm(
          `⚠️ 注意：模型 "${r.model_name}" 的新价 ${n} 是全局价 ${globalPrice} 的 ${(n / globalPrice).toFixed(1)} 倍，是否确认?`
        )
        if (!ok) return
      }
      payload.push({
        model_name: r.model_name,
        price: n,
        note: r.note?.trim() || undefined,
      })
    }

    setSaving(true)
    try {
      const res = await setUserModelPrices(userId, payload)
      if (res.success) {
        toast.success('专属模型价格已保存')
      } else {
        toast.error(res.message || '保存失败')
      }
    } catch (e: unknown) {
      toast.error((e as Error).message || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const availableModels = useMemo(
    () => allModels.filter((m) => !usedModels.has(m.model_name)),
    [allModels, usedModels]
  )

  return (
    <Card>
      <CardHeader className='pb-3'>
        <CardTitle className='text-sm'>专属模型按次价格</CardTitle>
        <CardDescription className='text-xs'>
          为该用户单独设置「按次计费」模型的价格，优先级最高（覆盖全局价）。最终扣费 = 此处价格 × 用户在该请求所走分组的倍率。仅按次计费的模型可在此配置。
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-3'>
        {loading ? (
          <p className='text-muted-foreground text-xs'>加载中...</p>
        ) : (
          <>
            {rows.length === 0 ? (
              <p className='text-muted-foreground text-xs'>
                暂无专属价格。请使用下方下拉选择一个模型添加。
              </p>
            ) : (
              <div className='space-y-2'>
                {rows.map((r, idx) => {
                  const globalPrice = modelPriceMap[r.model_name]
                  return (
                    <div
                      key={r.model_name}
                      className='flex items-start gap-2'
                    >
                      <div className='flex-1 min-w-0'>
                        <div className='truncate text-sm font-medium' title={r.model_name}>
                          {r.model_name}
                        </div>
                        {globalPrice !== undefined && (
                          <div className='text-muted-foreground text-xs'>
                            全局价：{globalPrice}
                          </div>
                        )}
                      </div>
                      <Input
                        type='number'
                        min='0'
                        step='0.0001'
                        placeholder='价格'
                        value={r.price}
                        onChange={(e) =>
                          handleChangeRow(idx, 'price', e.target.value)
                        }
                        className='h-8 w-24 text-sm'
                      />
                      <Input
                        placeholder='备注'
                        value={r.note}
                        onChange={(e) =>
                          handleChangeRow(idx, 'note', e.target.value)
                        }
                        className='h-8 w-32 text-sm'
                      />
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        onClick={() => handleRemoveRow(idx)}
                        className='h-8 w-8 p-0'
                        aria-label='删除'
                      >
                        <Trash2 className='h-4 w-4' />
                      </Button>
                    </div>
                  )
                })}
              </div>
            )}

            <div className='flex items-center gap-2 border-t pt-3'>
              <Select value={addingModel} onValueChange={setAddingModel}>
                <SelectTrigger className='h-8 flex-1 text-sm'>
                  <SelectValue placeholder='选择要覆盖价格的模型...' />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {availableModels.length === 0 ? (
                      <div className='text-muted-foreground px-2 py-1 text-xs'>
                        没有更多可选模型
                      </div>
                    ) : (
                      availableModels.map((m) => (
                        <SelectItem key={m.model_name} value={m.model_name}>
                          {m.model_name}
                          <span className='text-muted-foreground ml-2 text-xs'>
                            （全局：{m.model_price}）
                          </span>
                        </SelectItem>
                      ))
                    )}
                  </SelectGroup>
                </SelectContent>
              </Select>
              <Button
                type='button'
                size='sm'
                variant='outline'
                onClick={handleAdd}
                disabled={!addingModel}
              >
                <Plus className='mr-1 h-4 w-4' />
                添加
              </Button>
            </div>

            <div className='flex justify-end pt-2'>
              <Button
                type='button'
                size='sm'
                onClick={handleSave}
                disabled={loading || saving}
              >
                {saving ? '保存中...' : '保存专属价格'}
              </Button>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )
}
