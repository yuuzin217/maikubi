import React, { useRef, useState, useMemo } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import { model } from '../../wailsjs/go/models';
import './DiffViewer.css';

type DiffViewerProps = {
    diffLines: model.DiffLine[];
    isMatched: boolean;
    onIgnorePath?: (path: string) => void;
};

type SplitRow = {
    key: string;
    left: {
        lineNumber: number | null;
        status: 'matched' | 'deleted' | 'modified' | 'empty';
        text: string;
        jsonPath: string;
    };
    right: {
        lineNumber: number | null;
        status: 'matched' | 'added' | 'modified' | 'empty';
        text: string;
        jsonPath: string;
    };
};

export const DiffViewer: React.FC<DiffViewerProps> = ({ diffLines, isMatched, onIgnorePath }) => {
    const parentRef = useRef<HTMLDivElement>(null);
    const [viewMode, setViewMode] = useState<'split' | 'unified'>('split');

    // 左右対称のSplit行をメモリ上で高速再構成
    const splitRows = useMemo<SplitRow[]>(() => {
        const rows: SplitRow[] = [];
        let i = 0;
        
        while (i < diffLines.length) {
            const line = diffLines[i];
            const key = `row-${i}`;
            
            if (line.status === 'matched') {
                rows.push({
                    key,
                    left: { lineNumber: line.lineNumber, status: 'matched', text: line.text, jsonPath: line.jsonPath },
                    right: { lineNumber: line.lineNumber, status: 'matched', text: line.text, jsonPath: line.jsonPath },
                });
                i++;
            } else if (line.status === 'deleted') {
                // modified (値の変更) ペアの検出 (削除と追加が同じJSONPathで連続している場合)
                const nextLine = diffLines[i + 1];
                if (nextLine && nextLine.status === 'added' && nextLine.jsonPath === line.jsonPath) {
                    rows.push({
                        key,
                        left: { lineNumber: line.lineNumber, status: 'modified', text: line.text, jsonPath: line.jsonPath },
                        right: { lineNumber: nextLine.lineNumber, status: 'modified', text: nextLine.text, jsonPath: nextLine.jsonPath },
                    });
                    i += 2; // 2行消費
                } else {
                    // 単なる削除
                    rows.push({
                        key,
                        left: { lineNumber: line.lineNumber, status: 'deleted', text: line.text, jsonPath: line.jsonPath },
                        right: { lineNumber: null, status: 'empty', text: '', jsonPath: '' },
                    });
                    i++;
                }
            } else if (line.status === 'added') {
                // 単なる追加
                rows.push({
                    key,
                    left: { lineNumber: null, status: 'empty', text: '', jsonPath: '' },
                    right: { lineNumber: line.lineNumber, status: 'added', text: line.text, jsonPath: line.jsonPath },
                });
                i++;
            } else {
                // modified (直接返る場合のフォールバック)
                rows.push({
                    key,
                    left: { lineNumber: line.lineNumber, status: 'modified', text: line.text, jsonPath: line.jsonPath },
                    right: { lineNumber: line.lineNumber, status: 'modified', text: line.text, jsonPath: line.jsonPath },
                });
                i++;
            }
        }
        return rows;
    }, [diffLines]);

    // 表示アイテム数の決定
    const itemCount = viewMode === 'split' ? splitRows.length : diffLines.length;

    // TanStack Virtual v3 によるバーチャルスクロール
    const rowVirtualizer = useVirtualizer({
        count: itemCount,
        getScrollElement: () => parentRef.current,
        estimateSize: () => 24, // 各行の高さの目安(px)
        overscan: 30,           // 画面外に事前にレンダリングする行数
    });

    if (!diffLines || diffLines.length === 0) {
        return (
            <div className="diff-viewer-empty">
                <div className="empty-state-content">
                    <span className="empty-icon">🔍</span>
                    <p>No difference data. Please click "Run Diff" to start comparing APIs.</p>
                </div>
            </div>
        );
    }

    return (
        <div className="diff-viewer-wrapper">
            <div className="diff-viewer-header animate-fade-in">
                <div className="header-left">
                    <h3>Detailed Diff Comparison</h3>
                    <p className="subtitle">
                        {viewMode === 'split' ? 'Split View (Baseline vs Staging)' : 'Unified View (GitHub Style)'}
                    </p>
                </div>

                {/* ビューモード切り替えトグル */}
                <div className="view-mode-toggle">
                    <button 
                        className={`toggle-btn ${viewMode === 'split' ? 'active' : ''}`}
                        onClick={() => setViewMode('split')}
                        title="Compare Baseline and Staging side-by-side"
                    >
                        📊 Split View
                    </button>
                    <button 
                        className={`toggle-btn ${viewMode === 'unified' ? 'active' : ''}`}
                        onClick={() => setViewMode('unified')}
                        title="Display diff combined in a single column with + / -"
                    >
                        📝 Unified View
                    </button>
                </div>

                <div className={`match-badge ${isMatched ? 'matched' : 'degraded'}`}>
                    {isMatched ? (
                        <>
                            <span className="badge-icon">✓</span>
                            <span>Staging == Baseline (Success)</span>
                        </>
                    ) : (
                        <>
                            <span className="badge-icon">✗</span>
                            <span>Degradation Detected</span>
                        </>
                    )}
                </div>
            </div>

            <div
                ref={parentRef}
                className={`diff-viewer-container ${viewMode}`}
            >
                <div
                    style={{
                        height: `${rowVirtualizer.getTotalSize()}px`,
                        width: '100%',
                        position: 'relative',
                    }}
                >
                    {rowVirtualizer.getVirtualItems().map((virtualRow) => {
                        if (viewMode === 'split') {
                            // 📊 Split View (左右2画面)
                            const row = splitRows[virtualRow.index];
                            if (!row) return null;

                            const leftIsIgnorable = row.left.jsonPath && row.left.jsonPath !== "" && row.left.jsonPath !== "$";
                            const rightIsIgnorable = row.right.jsonPath && row.right.jsonPath !== "" && row.right.jsonPath !== "$";

                            return (
                                <div
                                    key={virtualRow.key}
                                    className="diff-split-row"
                                    style={{
                                        position: 'absolute',
                                        top: 0,
                                        left: 0,
                                        width: '100%',
                                        height: `${virtualRow.size}px`,
                                        transform: `translateY(${virtualRow.start}px)`,
                                    }}
                                >
                                    {/* 左ペイン (Baseline / 旧) */}
                                    <div className={`diff-pane left-pane ${row.left.status}`}>
                                        <span className="diff-line-num">
                                            {row.left.lineNumber !== null ? row.left.lineNumber : ''}
                                        </span>
                                        <pre className="diff-line-content">{row.left.text}</pre>
                                        
                                        {leftIsIgnorable && onIgnorePath && (
                                            <div className="diff-line-actions">
                                                <button
                                                    className="ignore-btn"
                                                    onClick={() => onIgnorePath(row.left.jsonPath)}
                                                    title={`Ignore: ${row.left.jsonPath}`}
                                                >
                                                    🚫 Ignore
                                                </button>
                                            </div>
                                        )}
                                    </div>

                                    {/* 右ペイン (Staging / 新) */}
                                    <div className={`diff-pane right-pane ${row.right.status}`}>
                                        <span className="diff-line-num">
                                            {row.right.lineNumber !== null ? row.right.lineNumber : ''}
                                        </span>
                                        <pre className="diff-line-content">{row.right.text}</pre>
                                        
                                        {rightIsIgnorable && onIgnorePath && (
                                            <div className="diff-line-actions">
                                                <button
                                                    className="ignore-btn"
                                                    onClick={() => onIgnorePath(row.right.jsonPath)}
                                                    title={`Ignore: ${row.right.jsonPath}`}
                                                >
                                                    🚫 Ignore
                                                </button>
                                            </div>
                                        )}
                                    </div>
                                </div>
                            );
                        } else {
                            // 📝 Unified View (GitHub風1画面)
                            const line = diffLines[virtualRow.index];
                            if (!line) return null;

                            const isIgnorable = line.jsonPath && line.jsonPath !== "" && line.jsonPath !== "$";
                            
                            // GitHub風プレフィックス (+ / -) の決定
                            const prefix = line.status === 'added' ? '+' : line.status === 'deleted' ? '-' : ' ';
                            
                            return (
                                <div
                                    key={virtualRow.key}
                                    className={`diff-line-row ${line.status}`}
                                    style={{
                                        position: 'absolute',
                                        top: 0,
                                        left: 0,
                                        width: '100%',
                                        height: `${virtualRow.size}px`,
                                        transform: `translateY(${virtualRow.start}px)`,
                                    }}
                                >
                                    <span className="diff-line-num">{line.lineNumber}</span>
                                    <span className={`diff-sign-prefix ${line.status}`}>{prefix}</span>
                                    <pre className="diff-line-content">{line.text}</pre>
                                    
                                    {isIgnorable && onIgnorePath && (
                                        <div className="diff-line-actions">
                                            <span className="jsonpath-tag" title={line.jsonPath}>
                                                {line.jsonPath}
                                            </span>
                                            <button
                                                className="ignore-btn"
                                                onClick={() => onIgnorePath(line.jsonPath)}
                                                title="Add this path to ignore list"
                                            >
                                                🚫 Ignore
                                            </button>
                                        </div>
                                    )}
                                </div>
                            );
                        }
                    })}
                </div>
            </div>
        </div>
    );
};
