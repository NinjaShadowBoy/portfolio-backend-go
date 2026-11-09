---
kind: publication
slug: cerc2026-audio
title: LinguoMT: Audio Preprocessing Strategies for Low-Resource African Language Speech Translation
lang: en
url: /publications
pdf: /papers/linguomt-audio-preprocessing-strategies.pdf
venue: Collaborative European Research Conference (CERC 2026), ATU Galway, Ireland
venue_fr: Collaborative European Research Conference (CERC 2026), ATU Galway, Irlande
date: 2026-10
authors: Philippe Tamla, Ruth Stephie Darelle Bilogui Tiguena, Alex Nelson Ryan Abena, Gladys Kouakam, Haithem Afli
role: 3rd author
summary_en: A controlled evaluation of four audio preprocessing strategies (direct, trimmed, normalized, chunk-based) on SeamlessM4T-v2 Large and the Whisper + NLLB pipeline, using noisy field recordings. Trimming silence improved Yoruba to English by 1.22 BLEU in the pilot (n=3), fixed-size chunking made results worse, and the cascade failed silently with empty outputs.
summary_fr: Une évaluation contrôlée de quatre stratégies de prétraitement audio (direct, silences retirés, normalisé, découpage fixe) sur SeamlessM4T-v2 Large et la chaîne Whisper + NLLB, à partir d'enregistrements de terrain bruités. Le retrait des silences améliore le yoruba vers l'anglais de 1,22 BLEU dans le pilote (n=3), le découpage fixe dégrade les résultats, et la cascade échoue en silence avec des sorties vides.
reviewed: false
---

# LinguoMT: Audio Preprocessing Strategies for Low-Resource African Language Speech Translation

## Abstract

Audio preprocessing remains a critical yet insufficiently studied component of multilingual speech translation, particularly for low-resource African languages collected under heterogeneous field conditions. This paper presents LinguoMT-Audio, a controlled evaluation of four preprocessing strategies (Direct, Trimmed, Normalized, and Chunk-based) applied to SeamlessM4T-v2 Large and a cascaded Whisper-large-v3 plus No Language Left Behind (NLLB)-distilled-600M pipeline using the African-Celtic corpus. Silence-aware trimming improves Yoruba translation by +1.22 Bilingual Evaluation Understudy (BLEU), whereas fixed chunking degrades performance and root-mean-square (RMS) normalization has marginal effect. The evaluated Whisper cascade produced empty ASR and translation outputs for every sample and strategy, identifying a systematic failure in the tested audio-to-transcript path whose precise cause is not isolated by the present design. A lightweight preprocessing heuristic based on silence ratio is proposed and preliminarily evaluated.

Keywords: African languages, Speech Translation, Automatic Speech Recognition, Audio Preprocessing

## Introduction

Recent advances in multilingual speech foundation models have substantially improved automatic speech translation (AST) through large-scale architectures such as SeamlessM4T-v2 Large (Barrault et al., 2023) and Whisper (Radford et al., 2023). These systems combine self-supervised acoustic representation learning, multilingual transfer learning, and sequence-to-sequence modelling to support speech understanding across diverse linguistic environments.

Nevertheless, their effectiveness remains highly dependent on the acoustic compatibility between inference audio and the distributions encountered during pre-training. Variations in sampling frequency, silence structure, loudness normalization, and recording quality can significantly degrade downstream translation performance (Watanabe et al., 2018). This challenge is especially critical for African languages, where speech recordings are often collected under uncontrolled field conditions involving environmental noise, irregular pauses, and heterogeneous recording devices. While multilingual speech translation (ST) research has primarily focused on model scaling and multilingual transfer learning (Barrault et al., 2023; Radford et al., 2023), comparatively limited attention has been given to the isolated impact of preprocessing on translation quality. Existing studies generally adopt fixed preprocessing pipelines without systematically analysing how acoustic variability influences encoder stability and multilingual inference behaviour. This limitation is particularly relevant for African languages, where tonal structures and language-dependent silence distributions may interact differently with trimming, normalization, and segmentation strategies. Exploratory analysis of the African-Celtic corpus (IWSLT, 2026) reveals substantial acoustic variability: Yoruba utterances exhibit a silence ratio of 0.486 compared to 0.326 for Igbo, suggesting that preprocessing behaviour may differ significantly across linguistic settings. Three problem statements motivate this study. PS1: Existing multilingual speech translation research has not systematically compared preprocessing strategies for African languages under controlled experimental settings, leaving practitioners without evidence-based guidance for heterogeneous recordings. PS2: The relationship between acoustic properties such as silence ratio and preprocessing effectiveness has not been quantitatively modelled, limiting the development of rapid exploratory data analysis (EDA)-based preprocessing heuristics. PS3: Cascade-based speech translation systems may fail silently at the audio-to-transcript stage, yet preprocessing studies rarely distinguish empty-output failures from valid but low-quality recognition results, creating operational and evaluation risks. To address these challenges, this study investigates two research questions. RQ1: Which preprocessing strategy among Direct, Trimmed, Normalized, and Chunk-based most consistently improves speech-translation quality, measured by BLEU and ChrF, for the evaluated African languages, and what baseline ASR limitations are revealed by WER, CER, and empty-output analysis for the two architectures? RQ2: To what extent does language-level silence ratio predict the translation benefit obtained from silence trimming, and can this relationship support the formulation of an EDA-based preprocessing selection rule without requiring full model evaluation? This study complements a shared zero-shot benchmark and a separate cascade-versus-end-to-end error analysis (Tamla et al., 2026a; 2026c) by isolating audio-preprocessing effects. Following Nunamaker et al. (1991), Theory Building defines the preprocessing framework and silence-ratio hypothesis; System Development implements four pipelines with two architectures; and Observation and Experimentation evaluates their language-specific behaviour. Section 2 derives requirements, Section 3 describes the design, Section 4 reports results, and Section 5 concludes.

## Related Work

Audio preprocessing reduces mismatch before model inference through resampling, voice activity detection, normalization, and segmentation (Ramirez et al., 2007; Watanabe et al., 2018). Whisper uses fixed-duration representations (Radford et al., 2023), while SeamlessM4T learns multilingual speech representations (Barrault et al., 2023). FLEURS, AfriSpeech-200, and BibleTTS broaden African and multilingual speech coverage (Conneau et al., 2022; Olatunji et al., 2023; Meyer et al., 2022), but they do not provide controlled comparisons of preprocessing strategies for African speech translation. The first remaining challenge (RC1) is the absence of controlled, language-stratified evaluation of preprocessing strategies for African multilingual speech translation under heterogeneous acoustic conditions. This gap is particularly relevant for tonal languages, where acoustic structure carries lexical information. Yoruba tone realisation studies show that fundamental frequency patterns are central to tone modelling in continuous speech (van Niekerk and Barnard, 2012; 2014). Consequently, preprocessing decisions such as silence trimming or segmentation may affect not only acoustic efficiency but also the preservation of tone-sensitive speech boundaries. While amplitude normalisation can reduce recording-level variation, it does not directly address tonal structure. The second RC2 is the limited understanding of how silence-aware preprocessing interacts with tonal-language translation performance, especially when A further challenge (RC3) concerns the reliability and diagnosability of the audio interface in cascaded systems. Multilingual ASR models generally assume standardized waveforms and fixed feature-extraction settings, while African speech corpora may contain heterogeneous sampling rates, recording devices, and language metadata. The shared pipeline used in this study standardizes audio at 16 kHz before inference. Nevertheless, an ASR component can still return an empty transcription without raising an exception, preventing downstream translation and making ordinary quality metrics misleading. The third remaining challenge is therefore the insufficient reporting and diagnosis of silent audio-to-transcript failures in cascade-based multilingual speech translation. Addressing RC1, RC2, and RC3 requires a controlled framework that compares preprocessing strategies while separately reporting translation quality, ASR quality, and output validity.

## Design

Figure 1 models the controlled workflow addressing RC1. The researcher loads African-Celtic data, selects a language and one mutually exclusive preprocessing strategy, configures SeamlessM4T-v2 or Whisper+NLLB, runs inference, and evaluates outputs. BLEU and ChrF are computed per strategy; WER and CER come from a separate architecture- and language-level ASR pass. The researcher then compares strategies and derives recommendations. Numbers 1-4 in Figure 1 indicate presentation order, not sequential execution.

Figure 1: Use case diagram of the proposed audio preprocessing evaluation framework.

Figure 2 illustrates the component architecture of the preprocessing and speech translation pipeline, addressing RC3 by making the audio-standardization and output-validation stages explicit. The framework consists of five layers: input, audio preprocessing, model inference, evaluation, and analysis. The input layer contains heterogeneous 48 kHz African-Celtic recordings with variable silence, amplitude, and duration. The shared extraction path converts all waveforms to 16 kHz before applying the selected strategy. Direct preprocessing performs no optional transformation beyond this standardization. Trimmed preprocessing removes silence, Normalized preprocessing applies RMS amplitude normalization, and Chunk-based preprocessing segments the waveform into fixed windows. The inference layer evaluates SeamlessM4T-v2 Large and the Whisper-large-v3 plus NLLB-distilled-600M cascade (NLLB Team et al., 2022). The evaluation layer computes BLEU and ChrF for translation, reports baseline WER and CER for ASR, and records output validity so that empty predictions are distinguished from scored outputs.

Figure 2: Component architecture of the proposed audio preprocessing and speech translation pipeline.

### Preprocessing Strategies

All four strategies use torchaudio and librosa and share 16 kHz extraction before optional transformations; resampling is therefore an implementation prerequisite, not an evaluated strategy. African-Celtic mean silence ratios are 0.486 for Yoruba and 0.326 for Igbo, with mean durations of approximately 10.6-12.6 seconds.

## Experimental Protocol

SeamlessM4T covers Igbo and Yoruba; Whisper+NLLB covers Yoruba and Hausa. The exploratory debug run uses three development samples per language. BLEU and ChrF assess strategy-level translation. A separate baseline ASR pass reports WER/CER by architecture and language; because its files contain no strategy field, these values cannot support strategy-level ASR comparisons. Gold-transcript text translation provides an upper bound that separates translation capacity from acoustic error. Silence ratio and RMS energy support analysis of preprocessing effects.

## Results and Discussion

### Text-Path Baseline

Table 1 gives the text-path ceiling. SeamlessM4T reaches 30.70 BLEU for Igbo-to-English and 14.89 for Yoruba-to-English; Whisper+NLLB reaches 30.31 for Hausa-to-English and 12.72 for Yoruba-to-English. Meaningful gold-transcript results, contrasted with weaker audio paths, identify acoustic processing as the principal observed bottleneck.

Table 1: Text-path translation baseline (gold transcript input). BLEU and ChrF computed with SacreBLEU (Post, 2018).

| Architecture | Language | Direction | BLEU | ChrF |
|---|---|---|---|---|
| SeamlessM4T | Igbo | Igbo→EN | 30.70 | 56.35 |
| SeamlessM4T | Igbo | EN→Igbo | 23.74 | 51.24 |
| SeamlessM4T | Yoruba | Yoruba→EN | 14.89 | 42.46 |
| SeamlessM4T | Yoruba | EN→Yoruba | 2.72 | 22.12 |
| Whisper+NLLB | Yoruba | Yoruba→EN | 12.72 | 43.44 |
| Whisper+NLLB | Yoruba | EN→Yoruba | 4.69 | 32.17 |
| Whisper+NLLB | Hausa | Hausa→EN | 30.31 | 54.67 |
| Whisper+NLLB | Hausa | EN→Hausa | 16.17 | 46.71 |

### SeamlessM4T Audio Preprocessing Comparison

Table 2 compares the four strategies. Yoruba-to-English improves from 15.50 BLEU (Direct) to 16.72 (Trimmed), consistent with its 0.486 silence ratio. Igbo-to-English falls from 3.38 to 1.64, while English-to-Igbo rises from 0.67 to 7.14, indicating directional sensitivity. Normalization changes neither language, and Chunk-based processing reduces Yoruba-to-English to 7.31. improvement for either language (15.50 vs 15.50 for Yoruba to English), consistent with the hypothesis that modern multilingual encoders already provide robustness to moderate amplitude variation through learned acoustic representations. Chunk-based segmentation substantially degrades performance for Yoruba (15.50 Direct to 7.31 Chunk-based), as fixed 15-second windows cause contextual fragmentation when utterances approach segment boundaries.

Table 2: SeamlessM4T BLEU across audio preprocessing strategies (African-Celtic, n=3 per language).

| Language | Direction | Direct | Trimmed | Normalized | Chunk-based |
|---|---|---|---|---|---|
| Igbo | Igbo→EN | 3.38 | 1.64 | 3.38 | 2.31 |
| Igbo | EN→Igbo | 0.67 | 7.14 | 0.67 | 0.70 |
| Yoruba | Yoruba→EN | 15.50 | 16.72 | 15.50 | 7.31 |
| Yoruba | EN→Yoruba | 3.23 | 3.24 | 3.23 | 0.51 |

### Whisper+NLLB Cascade Behaviour

Whisper+NLLB produced no valid transcription or translation for any Yoruba or Hausa sample under any strategy. These are pipeline failures, not zero-quality translations, and are therefore not tabulated as BLEU. Table 1 indicates that NLLB translates valid transcripts, localising the failure to the audio-to-transcript stage or its integration; its precise cause is not isolated.

### Automatic Speech Recognition Results

Table 3 reports baseline ASR by architecture and language, not strategy. SeamlessM4T-v2 produces three valid outputs per language; WER may exceed 1.0 when insertions outnumber reference words. Whisper produces no valid Yoruba or Hausa transcription, so WER/CER are omitted rather than misrepresented as ordinary scores of 1.0.

Table 3: Baseline ASR results on African-Celtic (n=3 per language). Lower WER and CER are better; dashes denote that no valid transcription was produced.

| Architecture | Language | Valid outputs | WER | CER |
|---|---|---|---|---|
| SeamlessM4T-v2 | Igbo | 3/3 | 1.014 | 0.524 |
| SeamlessM4T-v2 | Yoruba | 3/3 | 1.011 | 0.410 |
| Whisper+NLLB | Yoruba | 0/3 | -- | -- |
| Whisper+NLLB | Hausa | 0/3 | -- | -- |

### Acoustic Properties and Preprocessing Effectiveness

Table 4 relates acoustic properties to RQ2. Yoruba's higher silence ratio (0.486 versus Igbo's 0.326) coincides with its +1.22 BLEU trimming gain. This motivates, but does not validate, a pilot heuristic prioritising trimming above approximately 0.40 silence ratio. RMS variation is modest, consistent with normalization's limited effect.

Table 4: Acoustic properties of African-Celtic audio by language (mean across corpus).

| Language | Silence ratio | Mean RMS | Mean duration (s) | Preprocessing benefit |
|---|---|---|---|---|
| Yoruba | 0.486 | 0.097 | 12.6 | High (trimming +1.22 BLEU) |
| Igbo | 0.326 | 0.097 | 10.6 | Low (direction-dependent) |
| Hausa | 0.397 | 0.089 | 11.3 | Not measurable (cascade failure) |

## Discussion

Results suggest language- and direction-dependent preprocessing effects: trimming benefits Yoruba-to-English, normalization has no measurable effect, and fixed chunking loses context. Whisper+NLLB's empty-output failure underscores the need to validate ASR before translation. Because all strategies use 16 kHz extraction and 16 kHz FLEURS runs show the same pattern, sampling-rate mismatch is not established; language prompting, audio handling, or model configuration require controlled ablation. The silent failure mode of Whisper under these conditions, producing empty string outputs rather than raising an exception, is particularly dangerous in production deployments, as it may remain undetected without systematic validation. This finding directly addresses RC3 and establishes a concrete operational requirement for multilingual speech translation systems processing heterogeneous African field recordings. The three evaluated languages, Igbo, Yoruba, and Hausa, belong to the Niger-Congo and Afro-Asiatic families, which are among the more widely studied African language families in speech technology research. Whether the observed silence-ratio effects transfer to more typologically distant families such as Nilo-Saharan or Khoisan languages, which can involve different phonation types, click consonants, or distinct prosodic and pause structures, is an open question that the present exploratory study cannot resolve. Because the proposed silence-ratio heuristic is grounded in acoustic pause structure rather than family-specific phonological features, there is some reason to expect qualitative transferability, but this remains an assumption rather than an established result. Extending the evaluation to corpora covering additional African language families, as discussed under external validity below, would be required before the heuristic could be generalised beyond the languages examined here.

## Threats to Validity

Several limitations should be considered when interpreting these results. Regarding internal validity, the evaluation relies on debug-mode execution using three audio samples per language, which limits statistical power. The ASR pass was aggregated by architecture and language and was not repeated for each preprocessing strategy; WER and CER therefore diagnose baseline recognition quality but cannot support strategy-level ASR comparisons. Preprocessing hyperparameters were fixed rather than tuned per language.

Regarding external validity, the strategy analysis uses the African-Celtic corpus and three languages, limiting generalisation. Regarding construct validity, BLEU may underestimate translation quality for morphologically rich languages, while empty outputs make WER and CER uninformative unless output validity is reported separately. All conclusions must therefore be interpreted as exploratory.

## Conclusion

This exploratory Nunamaker-based study compared four audio preprocessing strategies for African speech translation, addressing controlled comparison (RC1), silence-aware tonal processing (RC2), and explicit diagnosis of cascade failures (RC3). Trimming improved Yoruba-to-English by +1.22 BLEU; normalization had no measurable benefit; and fixed chunking degraded performance. Whisper+NLLB produced no valid ASR or translation output, an operational failure rather than valid zero-quality translation. These pilot findings require larger-scale confirmation. Future work will repeat strategy-specific ASR evaluation at scale and ablate sampling rate, language prompting, feature extraction, and model configuration. Additional African language families, FLEURS, and AfriSpeech-200 will test the silence-ratio heuristic beyond African-Celtic.
