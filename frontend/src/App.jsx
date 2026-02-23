import React, { useState, useRef } from 'react'
import axios from 'axios'

const CHUNK_SIZE = 10 * 1024 * 1024 // 10MB
const MAX_BYTES = 10 * 1024 * 1024 * 1024 // 10GB
const API_BASE = 'http://localhost:8080'
const CONCURRENCY = 4
const MAX_RETRIES = 3

function formatBytes(b) {
  if (b === 0) return '0 B'
  const k = 1024
  const sizes = ['B','KB','MB','GB','TB']
  const i = Math.floor(Math.log(b) / Math.log(k))
  return parseFloat((b / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

export default function App(){
  const [status, setStatus] = useState('Idle')
  const [progress, setProgress] = useState(0)
  const [fileURL, setFileURL] = useState('')
  const [isUploading, setUploading] = useState(false)
  const [isPaused, setPaused] = useState(false)

  const uploadState = useRef({
    uploadId: null,
    key: null,
    totalChunks: 0,
    completedParts: [],
    queue: [],
    controllers: {},
  })

  const startWorkers = () => {
    for (let i=0;i<CONCURRENCY;i++) uploadNext()
  }

  const uploadNext = async () => {
    if (isPaused) return
    const s = uploadState.current
    const part = s.queue.shift()
    if (!part) return
    await uploadPart(part)
    if (s.queue.length > 0) {
      uploadNext()
    }
  }

  const uploadPart = async (partNumber) => {
    const s = uploadState.current
    const start = (partNumber-1) * CHUNK_SIZE
    const end = Math.min(start + CHUNK_SIZE, s.fileSize)
    const chunk = s.file.slice(start, end)

    let attempt = 0
    while (attempt <= MAX_RETRIES) {
      if (isPaused) {
        // requeue and stop
        s.queue.unshift(partNumber)
        return
      }
      try {
        // get presigned url
        const presign = await axios.post(`${API_BASE}/presign-part`, { uploadId: s.uploadId, key: s.key, partNumber })
        const url = presign.data.url

        const controller = new AbortController()
        s.controllers[partNumber] = controller

        const res = await fetch(url, { method: 'PUT', body: chunk, signal: controller.signal, headers: { 'Content-Type': s.file.type || 'application/octet-stream' } })
        if (!res.ok) throw new Error('Upload failed status ' + res.status)
        const etag = res.headers.get('etag') || ''
        s.completedParts.push({ ETag: etag, PartNumber: partNumber })
        delete s.controllers[partNumber]

        const done = s.completedParts.length
        setProgress(Math.round((done / s.totalChunks) * 100))
        if (done === s.totalChunks) await finalize()
        return
      } catch (err) {
        attempt++
        if (attempt > MAX_RETRIES) {
          setStatus('Failed uploading part ' + partNumber)
          setUploading(false)
          return
        }
        // backoff
        await new Promise(r => setTimeout(r, 500 * attempt))
      }
    }
  }

  const finalize = async () => {
    const s = uploadState.current
    // sort parts
    s.completedParts.sort((a,b) => a.PartNumber - b.PartNumber)
    await axios.post(`${API_BASE}/complete-multipart`, { key: s.key, uploadId: s.uploadId, parts: s.completedParts })
    setStatus('Upload finished')
    setUploading(false)
    setPaused(false)
    setFileURL(s.fileURL || '')
  }

  const handleFile = async (e) => {
    const file = e.target.files[0]
    if (!file) return
    if (file.size > MAX_BYTES) { setStatus('File too large (max 10GB)'); return }
    setStatus('Initializing...')
    try {
      const startRes = await axios.post(`${API_BASE}/initiate-multipart`, { key: file.name, size: file.size })
      const { uploadId, key, url } = startRes.data
      uploadState.current = { ...uploadState.current, uploadId, key, file, fileSize: file.size, fileURL: url, totalChunks: Math.ceil(file.size / CHUNK_SIZE), completedParts: [], queue: [] , controllers: {} }
      const total = uploadState.current.totalChunks
      uploadState.current.queue = Array.from({length: total}, (_,i) => i+1)
      setUploading(true)
      setPaused(false)
      setStatus(`Uploading ${total} parts...`)
      setProgress(0)
      startWorkers()
    } catch (err) {
      console.error(err)
      setStatus('Failed to initiate upload')
    }
  }

  const handlePause = () => {
    setPaused(true)
    setStatus('Paused')
    const s = uploadState.current
    // abort ongoing
    Object.values(s.controllers).forEach(ctrl => { try { ctrl.abort() } catch (e) {} })
  }

  const handleResume = () => {
    setPaused(false)
    setStatus('Resuming...')
    startWorkers()
  }

  return (
    <div className="app">
      <h2>Direct S3 Chunked Uploader (10GB)</h2>
      <input type="file" onChange={handleFile} />
      <div style={{marginTop:12}}>
        <div className="progress"><div className="progress-bar" style={{width: progress + '%'}}/></div>
        <div className="controls">
          {!isUploading && <div className="meta">{status}</div>}
          {isUploading && !isPaused && <button onClick={handlePause}>Pause</button>}
          {isUploading && isPaused && <button onClick={handleResume}>Resume</button>}
        </div>
        <div className="meta">{progress}%</div>
        {fileURL && <div className="meta">Uploaded: <a href={fileURL} target="_blank" rel="noreferrer">{fileURL}</a></div>}
      </div>
    </div>
  )
}
