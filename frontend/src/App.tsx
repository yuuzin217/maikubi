import { useState, useEffect } from 'react';
import './App.css';
import { SendDiffRequest, RecomputeDiff, GetProtoMessages, GetProtoMessageTemplate, GetProtoSchemaFields } from "../wailsjs/go/main/App";
import { DiffViewer } from "./components/DiffViewer";
import { service } from "../wailsjs/go/models";

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
    manualIgnores: [],
    protoSchema: '',
    protoRequestType: '',
    protoResponseType: '',
    schemaFileName: ''
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
                    manualIgnores: parsed.manualIgnores ?? DEFAULT_CONFIG.manualIgnores,
                    protoSchema: parsed.protoSchema ?? DEFAULT_CONFIG.protoSchema,
                    protoRequestType: parsed.protoRequestType ?? DEFAULT_CONFIG.protoRequestType,
                    protoResponseType: parsed.protoResponseType ?? DEFAULT_CONFIG.protoResponseType,
                    schemaFileName: parsed.schemaFileName ?? DEFAULT_CONFIG.schemaFileName
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

    // Protobuf 用の追加ステート
    const [protoSchema, setProtoSchema] = useState(initial.protoSchema);
    const [protoRequestType, setProtoRequestType] = useState(initial.protoRequestType);
    const [protoResponseType, setProtoResponseType] = useState(initial.protoResponseType);
    const [schemaFileName, setSchemaFileName] = useState(initial.schemaFileName);
    const [isDragOver, setIsDragOver] = useState(false);
    const [errorMessage, setErrorMessage] = useState<string | null>(null);
    const [availableMessages, setAvailableMessages] = useState<string[]>([]);
    const [isManualRequest, setIsManualRequest] = useState(false);
    const [isManualResponse, setIsManualResponse] = useState(false);
    const [schemaFields, setSchemaFields] = useState<Record<string, service.ProtoField[]>>({});

    // .proto スキーマ変更時、自動的にメッセージ定義一覧を解析
    useEffect(() => {
        if (protoSchema) {
            GetProtoMessages(protoSchema)
                .then((msgs) => {
                    setAvailableMessages(msgs);
                })
                .catch((err) => {
                    console.error("Failed to parse proto schema:", err);
                });

            // 全メッセージのフィールド一覧を取得して保持
            GetProtoSchemaFields(protoSchema)
                .then((fieldsMap) => {
                    setSchemaFields(fieldsMap || {});
                })
                .catch((err) => {
                    console.error("Failed to parse proto schema fields:", err);
                    setSchemaFields({});
                });
        } else {
            setAvailableMessages([]);
            setSchemaFields({});
        }
    }, [protoSchema]);

    // 設定値が変更されたらローカルストレージへ自動保存する副作用
    useEffect(() => {
        const config = { 
            path, 
            method, 
            body, 
            targets, 
            manualIgnores,
            protoSchema,
            protoRequestType,
            protoResponseType,
            schemaFileName
        };
        localStorage.setItem(STORAGE_KEY, JSON.stringify(config));
    }, [path, method, body, targets, manualIgnores, protoSchema, protoRequestType, protoResponseType, schemaFileName]);

    // Request Message Type が変更されたとき、リクエストボディが空なら自動的にテンプレートを生成してプリフィル
    useEffect(() => {
        if (method === 'PROTO (POST)' && protoSchema && protoRequestType && !body.trim()) {
            GetProtoMessageTemplate(protoSchema, protoRequestType)
                .then((template) => {
                    setBody(template);
                })
                .catch((err) => {
                    console.error("Failed to auto-fill JSON template:", err);
                });
        }
    }, [protoRequestType, protoSchema, method]);

    const handleFillTemplate = async () => {
        if (!protoSchema || !protoRequestType) return;
        try {
            const template = await GetProtoMessageTemplate(protoSchema, protoRequestType);
            setBody(template);
        } catch (err: any) {
            console.error("Failed to generate JSON template:", err);
            setErrorMessage("Failed to generate JSON template: " + (err.toString() || 'Unknown error'));
        }
    };

    const handleTargetChange = (env: 'production' | 'staging' | 'baseline', value: string) => {
        setTargets((prev: any) => ({
            ...prev,
            [env]: value
        }));
    };

    const handleSend = async () => {
        setErrorMessage(null);

        // PROTO (POST) 選択時のクライアントサイド・バリデーション
        if (method === 'PROTO (POST)') {
            if (!protoSchema) {
                setErrorMessage('Please upload or drag & drop a .proto schema file before running.');
                return;
            }
            if (!protoRequestType.trim()) {
                setErrorMessage('Please enter the Request Message Type (e.g., UserRequest).');
                return;
            }
            if (!protoResponseType.trim()) {
                setErrorMessage('Please enter the Response Message Type (e.g., UserResponse).');
                return;
            }
            if (!body.trim()) {
                setErrorMessage('Please enter a JSON Request Body for Protobuf serialization.');
                return;
            }
        }

        setLoading(true);
        setDiffResponse(null);
        try {
            const actualMethod = method === 'PROTO (POST)' ? 'POST' : method;
            const isProto = method === 'PROTO (POST)';
            const schema = isProto ? protoSchema : '';
            const reqType = isProto ? protoRequestType : '';
            const respType = isProto ? protoResponseType : '';

            const result = await SendDiffRequest(
                actualMethod, 
                path, 
                body, 
                targets, 
                manualIgnores,
                schema,
                reqType,
                respType
            );
            setDiffResponse(result);

            // ターゲットレスポンスにエラーが含まれているかチェック
            const errs = result.responses
                ?.map((r: any, idx: number) => r.error ? `${['Production', 'Staging', 'Baseline'][idx]}: ${r.error}` : null)
                .filter(Boolean);

            if (errs && errs.length > 0) {
                setErrorMessage(errs.join(' | '));
            }
        } catch (err: any) {
            console.error(err);
            setErrorMessage(err.toString() || 'An unexpected error occurred during the request.');
        } finally {
            setLoading(false);
        }
    };

    // Protobuf スキーマファイル関連のハンドラー
    const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (file) {
            loadProtoFile(file);
        }
    };

    const loadProtoFile = (file: File) => {
        const reader = new FileReader();
        reader.onload = async (event) => {
            const text = event.target?.result as string;
            setProtoSchema(text);
            setSchemaFileName(file.name);
            
            try {
                const msgs = await GetProtoMessages(text);
                setAvailableMessages(msgs);
                
                const fieldsMap = await GetProtoSchemaFields(text);
                setSchemaFields(fieldsMap || {});

                // 自動初期選択: 'request'/'response' を含むメッセージを優先的にセット
                const reqMsg = msgs.find(m => m.toLowerCase().includes('request')) || msgs[0] || '';
                const respMsg = msgs.find(m => m.toLowerCase().includes('response')) || msgs[1] || msgs[0] || '';
                
                setProtoRequestType(reqMsg);
                setProtoResponseType(respMsg);
                setIsManualRequest(false);
                setIsManualResponse(false);
            } catch (err: any) {
                console.error("Failed to parse loaded proto file:", err);
                setErrorMessage("Failed to parse proto file: " + (err.toString() || 'Unknown error'));
            }
        };
        reader.readAsText(file);
    };

    const handleDragOver = (e: React.DragEvent) => {
        e.preventDefault();
        setIsDragOver(true);
    };

    const handleDragLeave = () => {
        setIsDragOver(false);
    };

    const handleDrop = (e: React.DragEvent) => {
        e.preventDefault();
        setIsDragOver(false);
        const file = e.dataTransfer.files?.[0];
        if (file) {
            loadProtoFile(file);
        }
    };

    const clearSchema = () => {
        setProtoSchema('');
        setSchemaFileName('');
        setProtoRequestType('');
        setProtoResponseType('');
        setAvailableMessages([]);
        setIsManualRequest(false);
        setIsManualResponse(false);
        setSchemaFields({});
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

    // フォーム入力変更ハンドラー (単一の JSON 文字列 body にシームレスに同期)
    const handleFieldChange = (path: string[], val: any) => {
        let parsed: any = {};
        try {
            parsed = body ? JSON.parse(body) : {};
        } catch (e) {
            parsed = {};
        }

        const updateNested = (obj: any, keys: string[], value: any): any => {
            if (keys.length === 0) return value;
            const [currentKey, ...restKeys] = keys;
            const nextObj = obj && typeof obj === 'object' ? { ...obj } : {};
            nextObj[currentKey] = updateNested(nextObj[currentKey], restKeys, value);
            return nextObj;
        };

        const updatedObj = updateNested(parsed, path, val);
        setBody(JSON.stringify(updatedObj, null, 4));
    };

    // フォーム入力の再帰レンダリングエンジン
    const renderFormFields = (fields: service.ProtoField[], path: string[], currentVal: any) => {
        if (!fields || fields.length === 0) {
            return <div className="no-fields-tip">No fields defined in this message.</div>;
        }

        return (
            <div className="form-fields-list">
                {fields.map((field) => {
                    const fieldPath = [...path, field.name];
                    const fieldVal = currentVal ? currentVal[field.name] : undefined;

                    if (field.isRepeated) {
                        return renderRepeatedField(field, fieldPath, fieldVal || []);
                    }

                    if (field.type === 'message') {
                        return renderMessageField(field, fieldPath, fieldVal || {});
                    }

                    return renderPrimitiveField(field, fieldPath, fieldVal);
                })}
            </div>
        );
    };

    const renderPrimitiveField = (field: service.ProtoField, path: string[], value: any) => {
        const id = path.join('.');
        
        return (
            <div key={id} className="form-field-row">
                <label className="form-field-label">
                    <span className="field-name">{field.name}</span>
                    <span className="field-type-badge">{field.type}</span>
                </label>
                <div className="form-field-input-wrapper">
                    {field.type === 'bool' ? (
                        <select
                            value={value === undefined ? 'false' : String(value)}
                            onChange={(e) => handleFieldChange(path, e.target.value === 'true')}
                            className="form-select"
                        >
                            <option value="true">true</option>
                            <option value="false">false</option>
                        </select>
                    ) : field.type === 'enum' ? (
                        <select
                            value={value || (field.enumValues?.[0] ?? '')}
                            onChange={(e) => handleFieldChange(path, e.target.value)}
                            className="form-select"
                        >
                            {field.enumValues?.map((ev) => (
                                <option key={ev} value={ev}>{ev}</option>
                            ))}
                        </select>
                    ) : field.type === 'number' ? (
                        <input
                            type="number"
                            value={value ?? ''}
                            onChange={(e) => handleFieldChange(path, e.target.value === '' ? undefined : Number(e.target.value))}
                            placeholder="0"
                            className="form-input"
                        />
                    ) : (
                        <input
                            type="text"
                            value={value ?? ''}
                            onChange={(e) => handleFieldChange(path, e.target.value)}
                            placeholder="Enter text..."
                            className="form-input"
                        />
                    )}
                </div>
            </div>
        );
    };

    const renderMessageField = (field: service.ProtoField, path: string[], value: any) => {
        const id = path.join('.');
        const nestedFields = schemaFields[field.typeName || ''] || [];
        
        return (
            <div key={id} className="form-field-nested-card">
                <div className="nested-card-header">
                    <span className="nested-field-name">📂 {field.name}</span>
                    <span className="nested-field-type">{field.typeName || 'message'}</span>
                </div>
                <div className="nested-card-body">
                    {renderFormFields(nestedFields, path, value)}
                </div>
            </div>
        );
    };

    const renderRepeatedField = (field: service.ProtoField, path: string[], values: any[]) => {
        const id = path.join('.');
        
        const handleAddElement = () => {
            let defaultVal: any = "";
            if (field.type === 'number') defaultVal = 0;
            if (field.type === 'bool') defaultVal = false;
            if (field.type === 'enum') defaultVal = field.enumValues?.[0] ?? "";
            if (field.type === 'message') defaultVal = {};
            
            handleFieldChange(path, [...values, defaultVal]);
        };

        const handleRemoveElement = (index: number) => {
            const nextVals = values.filter((_, i) => i !== index);
            handleFieldChange(path, nextVals);
        };

        const handleElementChange = (index: number, val: any) => {
            const nextVals = [...values];
            nextVals[index] = val;
            handleFieldChange(path, nextVals);
        };

        return (
            <div key={id} className="form-field-repeated">
                <div className="repeated-header">
                    <span className="repeated-field-name">📋 {field.name}</span>
                    <span className="repeated-field-type">repeated {field.type}</span>
                    <button
                        type="button"
                        className="add-repeated-item-btn"
                        onClick={handleAddElement}
                    >
                        ＋ Add Item
                    </button>
                </div>
                <div className="repeated-items-list">
                    {values.length === 0 ? (
                        <div className="empty-repeated-tip">No items. Click "＋ Add Item" to add.</div>
                    ) : (
                        values.map((val, idx) => {
                            const elementPath = [...path, String(idx)];
                            return (
                                <div key={idx} className="repeated-item-row">
                                    <span className="item-index">#{idx}</span>
                                    <div className="repeated-item-input-wrapper">
                                        {field.type === 'message' ? (
                                            <div className="repeated-nested-wrapper">
                                                {renderFormFields(schemaFields[field.typeName || ''] || [], elementPath, val)}
                                            </div>
                                        ) : field.type === 'bool' ? (
                                            <select
                                                value={String(val)}
                                                onChange={(e) => handleElementChange(idx, e.target.value === 'true')}
                                                className="form-select"
                                            >
                                                <option value="true">true</option>
                                                <option value="false">false</option>
                                            </select>
                                        ) : field.type === 'enum' ? (
                                            <select
                                                value={val || (field.enumValues?.[0] ?? '')}
                                                onChange={(e) => handleElementChange(idx, e.target.value)}
                                                className="form-select"
                                            >
                                                {field.enumValues?.map((ev) => (
                                                    <option key={ev} value={ev}>{ev}</option>
                                                ))}
                                            </select>
                                        ) : field.type === 'number' ? (
                                            <input
                                                type="number"
                                                value={val}
                                                onChange={(e) => handleElementChange(idx, e.target.value === '' ? 0 : Number(e.target.value))}
                                                className="form-input"
                                            />
                                        ) : (
                                            <input
                                                type="text"
                                                value={val}
                                                onChange={(e) => handleElementChange(idx, e.target.value)}
                                                className="form-input"
                                            />
                                        )}
                                    </div>
                                    <button
                                        type="button"
                                        className="remove-repeated-item-btn"
                                        onClick={() => handleRemoveElement(idx)}
                                    >
                                        🗑️
                                    </button>
                                </div>
                            );
                        })
                    )}
                </div>
            </div>
        );
    };

    // targetLabels no longer needed

    return (
        <div id="App">
            <header className="app-header">
                <h1>maikubi</h1>
                <p className="app-tagline">Supercharged API Regression Testing & Diff Client</p>
            </header>
            
            <div className="container">
                {/* エラーポップアップ通知 */}
                {errorMessage && (
                    <div className="error-popup animate-fade-in">
                        <div className="error-popup-content">
                            <span className="error-icon">⚠️</span>
                            <div className="error-message-text">
                                <strong>Request / Transcoding Failed</strong>
                                <p>{errorMessage}</p>
                            </div>
                        </div>
                        <button className="error-close-btn" onClick={() => setErrorMessage(null)}>×</button>
                    </div>
                )}

                {/* 共通設定・URL設定の2カラム */}
                <div className="config-grid">
                    {/* 共通設定セクション */}
                    <div className="section config-section">
                        <h3>Common Request Settings</h3>
                        <div className="input-group">
                            <select value={method} onChange={(e) => setMethod(e.target.value)} className="select">
                                <option value="GET">GET</option>
                                <option value="POST">POST</option>
                                <option value="PROTO (POST)">PROTO (POST)</option>
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

                        {/* Protobuf 動的スキーマローダー設定パネル */}
                        {method === 'PROTO (POST)' && (
                            <div className="proto-config-panel animate-slide-down">
                                <div className="proto-file-row">
                                    <div 
                                        className={`proto-dropzone ${isDragOver ? 'dragover' : ''} ${protoSchema ? 'loaded' : ''}`}
                                        onDragOver={handleDragOver}
                                        onDragLeave={handleDragLeave}
                                        onDrop={handleDrop}
                                    >
                                        {schemaFileName ? (
                                            <div className="file-loaded-info">
                                                <span className="file-icon">📄</span>
                                                <span className="file-name">{schemaFileName}</span>
                                                <button className="clear-file-btn" onClick={clearSchema}>×</button>
                                            </div>
                                        ) : (
                                            <label className="dropzone-label">
                                                <span>📂 Drag & Drop your <b>.proto</b> file here or <strong>Browse</strong></span>
                                                <input 
                                                    type="file" 
                                                    accept=".proto" 
                                                    onChange={handleFileChange} 
                                                    style={{ display: 'none' }} 
                                                />
                                            </label>
                                        )}
                                    </div>
                                </div>
                                <div className="proto-types-row">
                                    <div className="proto-type-input">
                                        <div className="proto-type-header">
                                            <label>Request Message Type</label>
                                            {availableMessages.length > 0 && (
                                                <button 
                                                    type="button" 
                                                    className="toggle-mode-btn"
                                                    onClick={() => setIsManualRequest(!isManualRequest)}
                                                >
                                                    {isManualRequest ? '📋 Select' : '✍️ Manual'}
                                                </button>
                                            )}
                                        </div>
                                        {availableMessages.length > 0 && !isManualRequest ? (
                                            <select 
                                                value={protoRequestType}
                                                onChange={(e) => setProtoRequestType(e.target.value)}
                                            >
                                                <option value="">-- Select Request Type --</option>
                                                {availableMessages.map(m => (
                                                    <option key={m} value={m}>{m}</option>
                                                ))}
                                            </select>
                                        ) : (
                                            <input 
                                                type="text" 
                                                value={protoRequestType}
                                                onChange={(e) => setProtoRequestType(e.target.value)}
                                                placeholder="e.g. UserRequest"
                                            />
                                        )}
                                    </div>
                                    <div className="proto-type-input">
                                        <div className="proto-type-header">
                                            <label>Response Message Type</label>
                                            {availableMessages.length > 0 && (
                                                <button 
                                                    type="button" 
                                                    className="toggle-mode-btn"
                                                    onClick={() => setIsManualResponse(!isManualResponse)}
                                                >
                                                    {isManualResponse ? '📋 Select' : '✍️ Manual'}
                                                </button>
                                            )}
                                        </div>
                                        {availableMessages.length > 0 && !isManualResponse ? (
                                            <select 
                                                value={protoResponseType}
                                                onChange={(e) => setProtoResponseType(e.target.value)}
                                            >
                                                <option value="">-- Select Response Type --</option>
                                                {availableMessages.map(m => (
                                                    <option key={m} value={m}>{m}</option>
                                                ))}
                                            </select>
                                        ) : (
                                            <input 
                                                type="text" 
                                                value={protoResponseType}
                                                onChange={(e) => setProtoResponseType(e.target.value)}
                                                placeholder="e.g. UserResponse"
                                            />
                                        )}
                                    </div>
                                </div>
                            </div>
                        )}

                        <div className="body-input-wrapper">
                            <div className="body-input-header">
                                <label className="body-label">
                                    {method === 'PROTO (POST)' ? 'Request Body Fields' : 'Request Body (JSON)'}
                                </label>
                            </div>

                            {method === 'PROTO (POST)' ? (
                                protoRequestType ? (
                                    <div className="form-editor-container animate-slide-down">
                                        {renderFormFields(
                                            schemaFields[protoRequestType] || [],
                                            [],
                                            (() => {
                                                try {
                                                    return body ? JSON.parse(body) : {};
                                                } catch (e) {
                                                    return {};
                                                }
                                            })()
                                        )}
                                    </div>
                                ) : (
                                    <div className="proto-form-placeholder animate-fade-in">
                                        💡 Please load a .proto schema and select/fill message types to display the input form.
                                    </div>
                                )
                            ) : (
                                <textarea 
                                    value={body} 
                                    onChange={(e) => setBody(e.target.value)} 
                                    placeholder="Request Body (JSON)"
                                    className="textarea"
                                    rows={6}
                                />
                            )}
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
