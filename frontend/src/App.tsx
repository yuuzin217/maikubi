import { useState, useEffect } from 'react';
import './App.css';
import { SendDiffRequest, RecomputeDiff } from "../wailsjs/go/main/App";
import { DiffViewer } from "./components/DiffViewer";

const formatJSON = (bodyStr: string) => {
    try {
        const parsed = JSON.parse(bodyStr);
        return JSON.stringify(parsed, null, 4);
    } catch (e) {
        return bodyStr;
    }
};

const STORAGE_KEY = 'maikubi_saved_config';

const DEFAULT_CONFIG = {
    path: '/api/v1/users',
    method: 'GET',
    body: '',
    targets: {
        production: 'http://localhost:8081',
        staging: 'http://localhost:8082',
        baseline: 'http://localhost:8083'
    },
    manualIgnores: []
};

function App() {
    // ローカルストレージから保存された設定を読み出すヘルパー
    const getInitialConfig = () => {
        try {
            const saved = localStorage.getItem(STORAGE_KEY);
            if (saved) {
                const parsed = JSON.parse(saved);
                return {
                    path: parsed.path ?? DEFAULT_CONFIG.path,
                    method: parsed.method ?? DEFAULT_CONFIG.method,
                    body: parsed.body ?? DEFAULT_CONFIG.body,
                    targets: parsed.targets ?? DEFAULT_CONFIG.targets,
                    manualIgnores: parsed.manualIgnores ?? DEFAULT_CONFIG.manualIgnores
                };
            }
        } catch (e) {
            console.error("Failed to load saved config from localStorage:", e);
        }
        return DEFAULT_CONFIG;
    };

    const initial = getInitialConfig();

    const [path, setPath] = useState(initial.path);
    const [method, setMethod] = useState(initial.method);
    const [body, setBody] = useState(initial.body);
    const [targets, setTargets] = useState(initial.targets);
    const [diffResponse, setDiffResponse] = useState<any>(null);
    const [manualIgnores, setManualIgnores] = useState<string[]>(initial.manualIgnores);
    const [newIgnorePath, setNewIgnorePath] = useState('');
    const [loading, setLoading] = useState(false);
    const [showRawResponses, setShowRawResponses] = useState(false);

    // 設定値が変更されたらローカルストレージへ自動保存する副作用
    useEffect(() => {
        const config = { path, method, body, targets, manualIgnores };
        localStorage.setItem(STORAGE_KEY, JSON.stringify(config));
    }, [path, method, body, targets, manualIgnores]);

    const handleTargetChange = (env: 'production' | 'staging' | 'baseline', value: string) => {
        setTargets((prev: any) => ({
            ...prev,
            [env]: value
        }));
    };

    const handleSend = async () => {
        setLoading(true);
        setDiffResponse(null);
        try {
            const result = await SendDiffRequest(method, path, body, targets, manualIgnores);
            setDiffResponse(result);
        } catch (err) {
            console.error(err);
        } finally {
            setLoading(false);
        }
    };

    // 差分行からダイレクトに Ignore を登録する処理
    const handleIgnorePath = async (ignorePath: string) => {
        if (manualIgnores.includes(ignorePath)) return;
        
        const nextIgnores = [...manualIgnores, ignorePath];
        setManualIgnores(nextIgnores);

        // APIリクエストの再送信なしで、フロントから渡された生のレスポンスをもとに瞬時に再計算
        if (diffResponse?.responses && diffResponse.responses.length >= 3) {
            try {
                const prodJSON = diffResponse.responses[0].body;
                const stagingJSON = diffResponse.responses[1].body;
                const baselineJSON = diffResponse.responses[2].body;
                
                const updated = await RecomputeDiff(prodJSON, stagingJSON, baselineJSON, nextIgnores);
                setDiffResponse((prev: any) => ({
                    ...prev,
                    diffLines: updated.diffLines,
                    isMatched: updated.isMatched
                }));
            } catch (err) {
                console.error("Failed to recompute diff:", err);
            }
        }
    };

    // Ignoreリストからパスを削除して再計算する処理
    const handleRemoveIgnore = async (ignorePath: string) => {
        const nextIgnores = manualIgnores.filter(p => p !== ignorePath);
        setManualIgnores(nextIgnores);

        if (diffResponse?.responses && diffResponse.responses.length >= 3) {
            try {
                const prodJSON = diffResponse.responses[0].body;
                const stagingJSON = diffResponse.responses[1].body;
                const baselineJSON = diffResponse.responses[2].body;
                
                const updated = await RecomputeDiff(prodJSON, stagingJSON, baselineJSON, nextIgnores);
                setDiffResponse((prev: any) => ({
                    ...prev,
                    diffLines: updated.diffLines,
                    isMatched: updated.isMatched
                }));
            } catch (err) {
                console.error("Failed to recompute diff:", err);
            }
        }
    };

    // 手動でインプットボックスから Ignore を追加
    const handleAddManualIgnore = () => {
        if (!newIgnorePath || manualIgnores.includes(newIgnorePath)) return;
        handleIgnorePath(newIgnorePath);
        setNewIgnorePath('');
    };

    // targetLabels no longer needed

    return (
        <div id="App">
            <header className="app-header">
                <h1>maikubi</h1>
                <p className="app-tagline">Supercharged API Regression Testing & Diff Client</p>
            </header>
            
            <div className="container">
                {/* 共通設定・URL設定の2カラム */}
                <div className="config-grid">
                    {/* 共通設定セクション */}
                    <div className="section config-section">
                        <h3>Common Request Settings</h3>
                        <div className="input-group">
                            <select value={method} onChange={(e) => setMethod(e.target.value)} className="select">
                                <option value="GET">GET</option>
                                <option value="POST">POST</option>
                            </select>
                            <input 
                                type="text" 
                                value={path} 
                                onChange={(e) => setPath(e.target.value)} 
                                placeholder="/api/v1/resource" 
                                className="path-input"
                            />
                            <button onClick={handleSend} disabled={loading} className="send-btn">
                                {loading ? 'Running...' : 'Run Diff'}
                            </button>
                        </div>
                        <div className="body-input-wrapper">
                            <textarea 
                                value={body} 
                                onChange={(e) => setBody(e.target.value)} 
                                placeholder='Request Body (JSON)'
                                className="textarea"
                                rows={4}
                            />
                        </div>
                    </div>

                    {/* ターゲット設定セクション */}
                    <div className="section targets-section">
                        <h3>Environment Base URLs</h3>
                        <div className="target-inputs">
                            {[
                                { key: 'production', label: 'Production' },
                                { key: 'staging', label: 'Staging' },
                                { key: 'baseline', label: 'Baseline' }
                            ].map(({ key, label }) => (
                                <div key={key} className="target-input-group">
                                    <label>{label}</label>
                                    <input 
                                        type="text" 
                                        value={targets[key as 'production' | 'staging' | 'baseline']} 
                                        onChange={(e) => handleTargetChange(key as 'production' | 'staging' | 'baseline', e.target.value)}
                                        placeholder="https://api.example.com"
                                    />
                                </div>
                            ))}
                        </div>
                    </div>
                </div>

                {/* Ignoreリスト管理セクション */}
                <div className="section ignore-section">
                    <div className="ignore-header">
                        <h3>Dynamic Field Ignore List (JSONPath)</h3>
                        <div className="ignore-input-group">
                            <input 
                                type="text"
                                value={newIgnorePath}
                                onChange={(e) => setNewIgnorePath(e.target.value)}
                                placeholder="e.g. $.data.id or $.items[*].timestamp"
                                className="ignore-input"
                                onKeyDown={(e) => {
                                    if (e.key === 'Enter') handleAddManualIgnore();
                                }}
                            />
                            <button className="add-ignore-btn" onClick={handleAddManualIgnore}>
                                ＋ Add Ignore
                            </button>
                        </div>
                    </div>
                    
                    {manualIgnores.length === 0 ? (
                        <p className="no-ignores-tip">
                            💡 Use the "🚫 Ignore" button on the diff view below to automatically skip timing-dependent fields (e.g. dynamic IDs, timestamps).
                        </p>
                    ) : (
                        <div className="ignore-chips-container">
                            {manualIgnores.map((p, idx) => (
                                <span key={idx} className="ignore-chip">
                                    <code className="chip-path">{p}</code>
                                    <button className="chip-remove-btn" onClick={() => handleRemoveIgnore(p)}>×</button>
                                </span>
                            ))}
                        </div>
                    )}
                </div>

                {/* 詳細な差分ビュー（Unified Diff / バーチャルスクロール搭載） */}
                {diffResponse && (
                    <DiffViewer 
                        diffLines={diffResponse.diffLines || []} 
                        isMatched={diffResponse.isMatched}
                        onIgnorePath={handleIgnorePath}
                    />
                )}

                {/* 生のレスポンス結果 (折りたたみ式) */}
                {diffResponse?.responses && (
                    <div className="raw-responses-collapsible">
                        <button 
                            className="toggle-raw-btn"
                            onClick={() => setShowRawResponses(!showRawResponses)}
                        >
                            {showRawResponses ? '▼ Hide Raw Responses' : '▶ Show Raw Responses'}
                        </button>
                        
                        {showRawResponses && (
                            <div className="results-grid animate-fade-in">
                                {diffResponse.responses.map((resp: any, i: number) => (
                                    <div key={i} className="response-column">
                                        <h4>{['Production', 'Staging', 'Baseline'][i]}</h4>
                                        <div className="response-box">
                                            {resp.error ? (
                                                <div className="error">{resp.error}</div>
                                            ) : (
                                                <>
                                                    <div className="status">{resp.status}</div>
                                                    <pre className="body-content">{formatJSON(resp.body)}</pre>
                                                </>
                                            )}
                                        </div>
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>
                )}
            </div>
        </div>
    );
}

export default App;
