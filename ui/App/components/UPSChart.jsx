import React, {useEffect, useState} from 'react';
import Panel from './Panel';

const format = value => value == null ? '—' : value.toFixed(1);
const stateNames = {active: 'Active', paused: 'Paused', collecting: 'Collecting samples', disconnected: 'RCON unavailable', unavailable: 'No fresh telemetry'};

export const seriesPath = (points, key, x, y) => {
    let connected = false;
    return points.map(point => {
        if (point[key] == null) {connected = false; return '';}
        const command = connected ? 'L' : 'M'; connected = true;
        return `${command}${x(point.time)},${y(point[key])}`;
    }).join(' ');
};

const UPSChart = ({running}) => {
    const [history, setHistory] = useState(null);
    const [error, setError] = useState('');
    const [selected, setSelected] = useState(null);
    useEffect(() => {
        let mounted = true;
        let controller;
        let timer;
        const refresh = async () => {
            controller = new AbortController();
            const timeout = setTimeout(() => controller.abort(), 7000);
            try {
                const response = await fetch('/api/server/ups', {credentials: 'same-origin', signal: controller.signal});
                if (!response.ok) throw new Error('UPS telemetry is temporarily unavailable. Retrying…');
                const data = await response.json();
                if (mounted) {setHistory(data); setError('');}
            } catch (e) {
                if (mounted) {setError('UPS telemetry is temporarily unavailable. Retrying…'); setHistory(null);}
            } finally {
                clearTimeout(timeout);
                if (mounted) timer = setTimeout(refresh, 10000);
            }
        };
        refresh();
        return () => {mounted = false; controller.abort(); clearTimeout(timer);};
    }, []);
    const points = history?.points || [];
    const current = points[points.length - 1];
    const active = points.filter(p => p.ups != null);
    const average = active.length ? active.reduce((sum,p) => sum + p.ups, 0) / active.length : null;
    const max = Math.ceil(Math.max(60, ...points.flatMap(p => [p.ups || 0, p.target || 0])) / 20) * 20;
    const x = time => 48 + (time - history.start) / 300 * 900;
    const y = value => 205 - value / max * 175;
    const hovered = selected == null ? null : points[selected];
    const state = !running ? 'Stopped' : (stateNames[current?.state] || 'Waiting for telemetry');
    return <Panel title="Performance · last 5 minutes" className="mt-6" content={<>
        <div className="flex flex-wrap gap-8 mb-4">
            {[['Current UPS', format(running ? current?.ups : null)], ['Target UPS', format(running ? current?.target : null)], ['5-minute average', format(average)], ['Players', running && current?.players != null ? current.players : '—']].map(([label,value]) =>
                <div key={label}><div className="text-sm">{label}</div><div className="text-2xl font-bold">{value}</div></div>)}
            <div className="ml-auto self-center text-orange" role="status">{state}</div>
        </div>
        {error ? <p role="alert">{error}</p> : !history ? <p>Loading UPS history…</p> : !history.configured ? <p>UPS telemetry is not configured.</p> : <>
            <svg viewBox="0 0 980 250" role="img" aria-label="Actual and target UPS over the last five minutes. Gaps indicate paused or unavailable samples." className="w-full" style={{minHeight: 180}}
                 onMouseMove={e => {const box=e.currentTarget.getBoundingClientRect(); setSelected(Math.max(0,Math.min(points.length-1,Math.round(((e.clientX-box.left)/box.width*980-48)/900*30))));}}
                 onMouseLeave={() => setSelected(null)}>
                <title>UPS over the last five minutes</title>
                {[0,1,2,3].map(i => {const v=max*i/3;return <g key={i}><line x1="48" x2="948" y1={y(v)} y2={y(v)} stroke="#666" strokeWidth="0.5"/><text x="38" y={y(v)+4} textAnchor="end" fill="#ccc" fontSize="12">{Math.round(v)}</text></g>;})}
                <text x="48" y="18" fill="#ccc" fontSize="12">UPS</text>
                <path d={seriesPath(points,'target',x,y)} fill="none" stroke="#aaa" strokeDasharray="6 4" strokeWidth="2"/>
                <path d={seriesPath(points,'ups',x,y)} fill="none" stroke="#e9a12a" strokeWidth="2.5"/>
                {points.filter(p => p.ups != null).map(p => <circle key={p.time} cx={x(p.time)} cy={y(p.ups)} r="2" fill="#e9a12a"/>)}
                {[0,1,2,3,4,5].map(i => <text key={i} x={48+i*180} y="230" textAnchor={i===0?'start':i===5?'end':'middle'} fill="#ccc" fontSize="12">{i===5?'Latest':`${5-i} min ago`}</text>)}
                {hovered && <line x1={x(hovered.time)} x2={x(hovered.time)} y1="30" y2="205" stroke="#eee" strokeDasharray="3 3"/>}
                {!active.length && <text x="498" y="115" textAnchor="middle" fill="#ddd" fontSize="16">No active UPS samples in this window</text>}
            </svg>
            <div className="flex flex-wrap justify-between gap-2 text-sm">
                <span><span style={{color:'#e9a12a'}}>● Actual UPS</span> · <span style={{color:'#aaa'}}>┄ Target UPS</span></span>
                <span>{hovered ? `${new Date(hovered.time*1000).toLocaleTimeString()} · ${format(hovered.ups)} UPS · target ${format(hovered.target)} · ${stateNames[hovered.state]}` : 'Refreshes every 10 seconds · hover to inspect'}</span>
            </div>
            <p className="text-sm mt-3">UPS measures game updates per second. The average excludes paused and unavailable samples.</p>
        </>}
    </>}/>;
};
export default UPSChart;
