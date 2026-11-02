import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import {
  ArrowLeft,
  Building2,
  CalendarDays,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Clock3,
  Info,
  MapPin,
  RefreshCw,
  Users,
  X,
} from "lucide-react"
import { chinaClock, chinaDate, datePlus } from "@/components/xique/dates"
import { localWorkspaceBridge, type LocalConnection } from "@/lib/mobile/local-school"
import type { Period } from "@/lib/api/academic"
import { useMobileBack } from "@/lib/mobile/back"
import { useContentTransition } from "@/hooks/use-content-transition"
import {
  classroomBridge,
  classroomError,
  currentPeriod,
  errorCode,
  groupRooms,
  PERIODS,
  roomStatus,
  sameQuery,
  STATUS,
  validateCatalog,
  validateDay,
  type ClassroomCatalog,
  type ClassroomDay,
  type ClassroomQuery,
} from "@/lib/mobile/classroom"
import "./classroom-prototype.css"

interface Props {
  workspaceId: string
  semester: string
  revision: number
  connection: LocalConnection | null
  close: () => void
  openSchool: () => void
}
type Selection = { campus: string; building: string }
const presets = [
  { id: "morning", label: "上午", start: 1, end: 5 },
  { id: "afternoon", label: "下午", start: 6, end: 10 },
  { id: "evening", label: "晚上", start: 11, end: 13 },
]
const pad = (n: number) => String(n).padStart(2, "0")
const periodText = (start: number, end: number) =>
  start === end ? `第 ${start} 节` : `第 ${start}–${end} 节`
function dateLabel(date: string) {
  return new Date(`${date}T00:00:00Z`).toLocaleDateString("zh-CN", {
    timeZone: "UTC",
    month: "long",
    day: "numeric",
    weekday: "short",
  })
}
function updatedAt(value: string) {
  return new Date(value).toLocaleString("zh-CN", {
    timeZone: "Asia/Shanghai",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  })
}
function selectionKey(workspace: string) {
  return `xique.classroom.building.${workspace}`
}
function savedSelection(workspace: string): Selection | null {
  try {
    const raw = JSON.parse(localStorage.getItem(selectionKey(workspace)) || "null")
    return raw && typeof raw.campus === "string" && typeof raw.building === "string" ? raw : null
  } catch {
    return null
  }
}

export function ClassroomPage({
  workspaceId,
  semester,
  revision,
  connection,
  close,
  openSchool,
}: Props) {
  const panel = useRef<HTMLDivElement>(null)
  useContentTransition(panel, "classroom", 0, true, true)
  const [date, setDate] = useState(chinaDate),
    [clock, setClock] = useState(chinaClock)
  const [catalog, setCatalog] = useState<ClassroomCatalog | null>(null),
    [selection, setSelection] = useState<Selection | null>(null)
  const [buildingOpen, setBuildingOpen] = useState(false),
    [detailId, setDetailId] = useState<string | null>(null)
  const [catalogBusy, setCatalogBusy] = useState(true),
    [busy, setBusy] = useState(false)
  const [catalogError, setCatalogError] = useState<unknown>(null),
    [queryError, setQueryError] = useState<unknown>(null)
  const [catalogVersion, setCatalogVersion] = useState(0),
    [refreshVersion, setRefreshVersion] = useState(0)
  const [result, setResult] = useState<ClassroomDay | null>(null),
    [periods, setPeriods] = useState<Period[]>([])
  const [start, setStart] = useState(1),
    [end, setEnd] = useState(13),
    [preset, setPreset] = useState("all")
  const rangeTouched = useRef(false),
    request = useRef(0),
    forcedRefresh = useRef(false)
  const generation = connection?.connection_generation
  useMobileBack(true, () => {
    if (detailId) setDetailId(null)
    else if (buildingOpen) setBuildingOpen(false)
    else close()
  })

  useEffect(() => {
    const tick = window.setInterval(() => setClock(chinaClock()), 30000)
    const key = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        if (detailId) setDetailId(null)
        else if (buildingOpen) setBuildingOpen(false)
        else close()
      }
    }
    window.addEventListener("keydown", key)
    return () => {
      clearInterval(tick)
      window.removeEventListener("keydown", key)
    }
  }, [buildingOpen, detailId, close])
  useEffect(() => {
    let active = true
    void localWorkspaceBridge
      .getSettings({ workspace_id: workspaceId, semester })
      .then((value) => {
        if (!active) return
        setPeriods(value.settings.periods)
        const now = currentPeriod(value.settings.periods, chinaClock())
        if (!rangeTouched.current && now !== null) {
          setStart(now)
          setEnd(now)
          setPreset("now")
        }
      })
      .catch(() => {
        /* Do not guess missing school times. */
      })
    return () => {
      active = false
    }
  }, [workspaceId, semester, revision])
  useEffect(() => {
    let active = true
    ++request.current
    void Promise.resolve()
      .then(() => {
        if (!active) return null
        setCatalogBusy(true)
        setCatalogError(null)
        setCatalog(null)
        setResult(null)
        setSelection(null)
        setDetailId(null)
        return classroomBridge.getBuildings({ workspace_id: workspaceId })
      })
      .then((raw) => {
        if (!active) return
        const value = validateCatalog(raw)
        if (generation !== undefined && value.connection_generation !== generation)
          throw { code: "CONNECTION_CHANGED" }
        setCatalog(value)
        const stored = savedSelection(workspaceId)
        if (
          stored &&
          value.campuses.some(
            (c) => c.id === stored.campus && c.buildings.some((b) => b.id === stored.building),
          )
        )
          setSelection(stored)
      })
      .catch((error) => {
        if (active) setCatalogError(error)
      })
      .finally(() => {
        if (active) setCatalogBusy(false)
      })
    return () => {
      active = false
    }
  }, [workspaceId, generation, catalogVersion])

  const campus = catalog?.campuses.find((c) => c.id === selection?.campus)
  const building = campus?.buildings.find((b) => b.id === selection?.building)
  const query = useMemo<ClassroomQuery | null>(
    () =>
      selection && catalog && building
        ? {
            workspace_id: workspaceId,
            semester,
            date,
            campus_id: selection.campus,
            building_id: selection.building,
          }
        : null,
    [workspaceId, semester, date, selection, catalog, building],
  )
  useEffect(() => {
    const id = ++request.current
    let active = true
    queueMicrotask(() => {
      if (!active) return
      setQueryError(null)
      setDetailId(null)
      setBusy(Boolean(query))
    })
    const force = forcedRefresh.current
    forcedRefresh.current = false
    const timer = window.setTimeout(() => {
      if (!query || !active) return
      void classroomBridge
        .getDay({ ...query, ...(force ? { refresh: true } : {}) })
        .then((raw) => {
          const value = validateDay(raw, query)
          if (!active || id !== request.current) return
          if (value.connection_generation !== catalog!.connection_generation)
            throw { code: "CONNECTION_CHANGED" }
          setResult(value)
        })
        .catch((error) => {
          if (!active || id !== request.current) return
          setQueryError(error)
          if (
            ![
              "OFFLINE_NO_CACHE",
              "UPSTREAM_UNAVAILABLE",
              "UPSTREAM_TLS_ERROR",
              "UPSTREAM_AUTH_REQUIRED",
              "UNRECOGNIZED_DOCUMENT",
            ].includes(errorCode(error))
          )
            setResult(null)
        })
        .finally(() => {
          if (active && id === request.current) setBusy(false)
        })
    }, 180)
    return () => {
      active = false
      clearTimeout(timer)
    }
  }, [query, catalog, refreshVersion])

  const shown =
    result &&
    query &&
    sameQuery(result, query) &&
    result.connection_generation === catalog?.connection_generation
      ? result
      : null
  const stale = Boolean(shown && (shown.cache.state === "STALE" || queryError))
  const groups = useMemo(() => groupRooms(shown?.rooms ?? [], start, end), [shown, start, end])
  const freeCount = shown?.rooms.filter((r) => roomStatus(r, start, end) === "FREE").length ?? 0
  const unknownCount =
    shown?.rooms.filter((r) => roomStatus(r, start, end) === "UNKNOWN").length ?? 0
  const detail = shown?.rooms.find((r) => r.id === detailId) ?? null
  const staleReason =
    shown?.cache.state === "STALE"
      ? shown.cache.reason
      : catalog?.cache.state === "STALE"
        ? catalog.cache.reason
        : null
  const error = queryError || catalogError || (staleReason ? { code: staleReason } : null)
  const info = error ? classroomError(error, connection?.connection_state === "CONNECTED") : null
  const now = currentPeriod(periods, clock),
    today = chinaDate()
  const totalBuildings = catalog?.campuses.reduce((n, c) => n + c.buildings.length, 0) ?? 0
  const refresh = useCallback(() => {
    if (!catalog || !query) setCatalogVersion((v) => v + 1)
    else {
      forcedRefresh.current = true
      setRefreshVersion((v) => v + 1)
    }
  }, [catalog, query])
  function selectBuilding(next: Selection) {
    ++request.current
    setSelection(next)
    setBuildingOpen(false)
    setDetailId(null)
    setQueryError(null)
    try {
      localStorage.setItem(selectionKey(workspaceId), JSON.stringify(next))
    } catch {
      /* Selection still works. */
    }
  }
  function chooseRange(id: string, from: number, to: number) {
    rangeTouched.current = true
    setPreset(id)
    setStart(from)
    setEnd(to)
  }
  function pickNow() {
    if (now !== null) {
      setDate(today)
      chooseRange("now", now, now)
    }
  }

  return (
    <div className="xm-classroom-overlay" role="dialog" aria-modal="true" aria-label="空闲教室">
      <div ref={panel} className="xm-classroom-page">
        <header className="xm-classroom-heading">
          <button className="xm-classroom-back" onClick={close} aria-label="返回校园">
            <ArrowLeft size={20} />
          </button>
          <div>
            <h1>Classroom</h1>
            <p className="xm-classroom-subtitle">找一间，安静自习。</p>
          </div>
          <span className="xm-classroom-source">河大教务</span>
        </header>
        <main className="xm-classroom-main">
          <section className="xm-classroom-filter" aria-label="空闲教室筛选">
            <button
              className="xm-classroom-building"
              onClick={() => setBuildingOpen(true)}
              disabled={catalogBusy || !catalog}
              aria-label="选择教学楼"
            >
              <span className="xm-filter-icon">
                <Building2 size={18} />
              </span>
              <span className="xm-building-copy">
                <small>{campus?.name || "教学楼"}</small>
                <strong>
                  {building?.name || (catalogBusy ? "正在读取教学楼…" : "选择教学楼")}
                </strong>
              </span>
              <ChevronDown size={18} className="xm-chevron" />
            </button>
            <div className="xm-classroom-date-nav">
              <button onClick={() => setDate((d) => datePlus(d, -1))} aria-label="前一天">
                <ChevronLeft size={18} />
              </button>
              <label className="xm-classroom-date">
                <CalendarDays size={17} />
                <span>{dateLabel(date)}</span>
                <input
                  type="date"
                  value={date}
                  aria-label="选择日期"
                  onChange={(e) => {
                    if (e.target.value) {
                      setDate(e.target.value)
                      if (preset === "now") setPreset("custom")
                    }
                  }}
                />
              </label>
              <button onClick={() => setDate((d) => datePlus(d, 1))} aria-label="后一天">
                <ChevronRight size={18} />
              </button>
            </div>
            <div className="xm-classroom-date-shortcuts">
              <button className={date === today ? "active" : ""} onClick={() => setDate(today)}>
                今天
              </button>
              <button
                className={date === datePlus(today, 1) ? "active" : ""}
                onClick={() => setDate(datePlus(today, 1))}
              >
                明天
              </button>
              <span className="xm-classroom-term">{catalog?.term.label}</span>
            </div>
            <div className="xm-classroom-periods">
              <div className="xm-period-chips">
                <span>查询节次</span>
                <button
                  className={preset === "now" && date === today ? "active" : ""}
                  disabled={now === null}
                  title={
                    now === null
                      ? "今天的课程时段已结束，可自行选择节次"
                      : "回到今天，选择当前或下一节课"
                  }
                  onClick={pickNow}
                >
                  现在
                </button>
                {presets.map((p) => (
                  <button
                    key={p.id}
                    className={preset === p.id ? "active" : ""}
                    onClick={() => chooseRange(p.id, p.start, p.end)}
                  >
                    {p.label}
                  </button>
                ))}
              </div>
              <div className="xm-period-range">
                <select
                  aria-label="起始节次"
                  value={start}
                  onChange={(e) => {
                    const n = Number(e.target.value)
                    chooseRange("custom", n, Math.max(n, end))
                  }}
                >
                  {PERIODS.map((p) => (
                    <option key={p} value={p}>
                      第 {p} 节
                    </option>
                  ))}
                </select>
                <span>至</span>
                <select
                  aria-label="结束节次"
                  value={end}
                  onChange={(e) => {
                    const n = Number(e.target.value)
                    chooseRange("custom", Math.min(start, n), n)
                  }}
                >
                  {PERIODS.map((p) => (
                    <option key={p} value={p}>
                      第 {p} 节
                    </option>
                  ))}
                </select>
              </div>
            </div>
          </section>
          {(catalogBusy || busy) && (
            <div className="xm-classroom-loading" role="status">
              <RefreshCw size={14} className="xm-room-spin" />
              {catalogBusy ? "正在读取学校目录…" : "正在查询教室…"}
            </div>
          )}
          {info && (
            <div className="xm-classroom-message" role="alert">
              <strong>{info.title}</strong>
              <p>{info.detail}</p>
              {info.action === "connect" || info.action === "settings" ? (
                <button onClick={openSchool}>
                  {info.action === "connect" ? "学校连接与更新" : "核对学期与作息"}
                </button>
              ) : info.action === "retry" ? (
                <button onClick={refresh}>重试查询</button>
              ) : null}
            </div>
          )}
          {!catalogBusy && !catalogError && catalog && !selection && (
            <div className="xm-classroom-message">
              <Building2 size={26} />
              <strong>{totalBuildings ? "先选择一栋教学楼" : "学校目录暂时没有教学楼"}</strong>
              <p>选择校区与教学楼，查看当天全部 13 节安排。</p>
              {totalBuildings > 0 && (
                <button onClick={() => setBuildingOpen(true)}>选择教学楼</button>
              )}
            </div>
          )}
          {catalog?.cache.state === "STALE" && !catalogError && (
            <p className="xm-classroom-stale">
              教学楼目录为上次查询结果 · {updatedAt(catalog.fetched_at)}
            </p>
          )}
          {shown && (
            <>
              <div className="xm-classroom-summary">
                <strong>
                  {dateLabel(shown.date)} · {periodText(start, end)}
                </strong>
                <span>{stale ? "上次查询" : `${freeCount} 间空闲 / ${shown.rooms.length} 间`}</span>
              </div>
              <div className="xm-classroom-data-status" role="status">
                <span>
                  {stale ? "上次查询" : shown.cache.state === "FRESH" ? "缓存结果" : "学校查询"} ·{" "}
                  {updatedAt(shown.fetched_at)}
                </span>
                <button onClick={refresh} disabled={busy} aria-label="刷新教室查询">
                  <RefreshCw size={13} />
                  刷新
                </button>
              </div>
              {stale && (
                <p className="xm-classroom-stale">
                  以下是上次查询的历史结果，请联网刷新后再确认是否空闲。
                </p>
              )}
              {shown.coverage.unknown_cells > 0 && (
                <p className="xm-classroom-unknown-note">
                  部分节次信息不完整，未知格不计入空闲。
                  {unknownCount > 0 ? `本时段 ${unknownCount} 间状态未知。` : ""}
                </p>
              )}
              <div className="xm-classroom-legend" aria-label="空闲教室状态图例">
                {Object.values(STATUS).map((s) => (
                  <span key={s.label}>
                    <i className={`xm-status-dot ${s.className}`} />
                    {s.label}
                  </span>
                ))}
              </div>
              {shown.rooms.length === 0 ? (
                <div className="xm-classroom-message">
                  <strong>这栋楼暂时没有可展示的教室</strong>
                  <p>这是学校目录返回的结果，可以更换教学楼再查询。</p>
                </div>
              ) : (
                <div
                  className={`xm-room-table${busy ? " is-loading" : ""}`}
                  aria-label="教室当天占用情况"
                  data-building={shown.building.id}
                  data-date={shown.date}
                  aria-busy={busy}
                >
                  <div className="xm-room-track-head">
                    <span>教室 · 13 节</span>
                    <div>
                      {PERIODS.map((p) => (
                        <span key={p}>{pad(p)}</span>
                      ))}
                    </div>
                  </div>
                  {groups.map((group) => (
                    <section className="xm-room-group" key={group.type} aria-label={group.type}>
                      <div className="xm-room-group-head">
                        <h2>{group.type}</h2>
                        <span>
                          {stale
                            ? `${group.rooms.length} 间`
                            : `${group.rooms.filter((r) => roomStatus(r, start, end) === "FREE").length} 间空闲`}
                        </span>
                      </div>
                      {group.rooms.map((room) => (
                        <button
                          key={room.id}
                          className="xm-room-card"
                          onClick={() => setDetailId(room.id)}
                          aria-label={`${room.name}，${room.seats === null ? "容量未提供" : room.seats + "座"}，本时段${STATUS[roomStatus(room, start, end)].label}`}
                        >
                          <span className="xm-room-copy">
                            <strong title={room.name}>{room.name}</strong>
                            <span>
                              <Users size={12} />
                              {room.seats === null ? "容量未提供" : room.seats + " 座"}
                            </span>
                          </span>
                          <span className="xm-room-track">
                            {PERIODS.map((p) => {
                              const s = room.cells.find((c) => c.period === p)?.status ?? "UNKNOWN"
                              const selected = p >= start && p <= end
                              return (
                                <i
                                  key={p}
                                  className={`xm-room-cell ${STATUS[s].className}${selected ? " is-selected" : ""}`}
                                  aria-label={`第 ${p} 节 ${STATUS[s].label}${selected ? "，当前查询时段" : ""}`}
                                />
                              )
                            })}
                          </span>
                        </button>
                      ))}
                    </section>
                  ))}
                </div>
              )}
            </>
          )}
          <p className="xm-classroom-disclaimer">
            <Info size={14} />
            <span>空闲依据教务查询，以现场为准。课表仍可离线查看。</span>
          </p>
        </main>
      </div>
      {buildingOpen && catalog && (
        <div
          className="xm-classroom-sheet-backdrop"
          role="presentation"
          onClick={() => setBuildingOpen(false)}
        >
          <section
            className="xm-classroom-sheet"
            role="dialog"
            aria-modal="true"
            aria-label="选择教学楼"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="xm-sheet-head">
              <div>
                <span>CAMPUS · BUILDING</span>
                <h2>选择教学楼</h2>
              </div>
              <button aria-label="关闭教学楼选择" onClick={() => setBuildingOpen(false)}>
                <X size={18} />
              </button>
            </div>
            {catalog.campuses.map((c) => (
              <div className="xm-campus-group" key={c.id}>
                <strong>
                  <MapPin size={14} />
                  {c.name}
                </strong>
                {c.buildings.length ? (
                  c.buildings.map((b) => (
                    <button
                      key={b.id}
                      className={
                        selection?.campus === c.id && selection.building === b.id ? "active" : ""
                      }
                      onClick={() => selectBuilding({ campus: c.id, building: b.id })}
                    >
                      <span>{b.name}</span>
                      {selection?.campus === c.id && selection.building === b.id && (
                        <span className="xm-selected-mark">当前</span>
                      )}
                    </button>
                  ))
                ) : (
                  <p className="xm-sheet-note">学校暂未提供教学楼。</p>
                )}
              </div>
            ))}
            <p className="xm-sheet-note">目录来自河南大学教务 · {updatedAt(catalog.fetched_at)}</p>
          </section>
        </div>
      )}
      {detail && shown && (
        <div
          className="xm-classroom-sheet-backdrop"
          role="presentation"
          onClick={() => setDetailId(null)}
        >
          <section
            className="xm-room-detail xm-classroom-sheet"
            role="dialog"
            aria-modal="true"
            aria-label={`${detail.name} 教室详情`}
            onClick={(e) => e.stopPropagation()}
          >
            <div className="xm-sheet-head">
              <div>
                <span>
                  {shown.campus.name} · {shown.building.name}
                </span>
                <h2>{detail.name}</h2>
              </div>
              <button aria-label="关闭教室详情" onClick={() => setDetailId(null)}>
                <X size={18} />
              </button>
            </div>
            <div className="xm-room-detail-meta">
              <span>{detail.type || "类型未标明"}</span>
              <span>
                <Users size={14} />
                {detail.seats === null ? "容量未提供" : detail.seats + " 座"}
              </span>
              <span>
                <Clock3 size={14} />
                {dateLabel(shown.date)}
              </span>
            </div>
            <div className="xm-room-detail-grid">
              {PERIODS.map((p) => {
                const state = detail.cells.find((c) => c.period === p)?.status ?? "UNKNOWN"
                const time = periods.find((t) => t.index === p && t.configured)
                return (
                  <div
                    key={p}
                    className={`xm-room-detail-cell ${STATUS[state].className}${p >= start && p <= end ? " is-selected" : ""}`}
                  >
                    <b>第 {p} 节</b>
                    {time && (
                      <small>
                        {time.start_time}–{time.end_time}
                      </small>
                    )}
                    <span>{STATUS[state].label}</span>
                  </div>
                )
              })}
            </div>
            <p className="xm-room-detail-note">
              {stale ? "上次查询" : "教务查询"} · {updatedAt(shown.fetched_at)}
              。空闲状态以现场为准。
            </p>
            <button className="xm-classroom-primary" onClick={() => setDetailId(null)}>
              返回教室列表
            </button>
          </section>
        </div>
      )}
    </div>
  )
}
