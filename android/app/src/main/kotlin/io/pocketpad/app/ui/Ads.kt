package io.pocketpad.app.ui

import android.app.Activity
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.viewinterop.AndroidView
import com.google.android.gms.ads.AdRequest
import com.google.android.gms.ads.AdSize
import com.google.android.gms.ads.AdView
import com.google.android.gms.ads.MobileAds
import com.google.android.ump.ConsentInformation
import com.google.android.ump.ConsentRequestParameters
import com.google.android.ump.UserMessagingPlatform
import io.pocketpad.app.BuildConfig
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import java.util.concurrent.atomic.AtomicBoolean

/**
 * Advertising, kept away from gameplay: the only ad is a banner on the
 * Settings screen. The SDK is initialised once consent allows it, and no ad
 * is ever shown or loaded on the controller screen, so it cannot add input
 * latency or invite accidental taps.
 */
object Ads {
    private val started = AtomicBoolean(false)
    private val _ready = MutableStateFlow(false)
    private val _privacyOptionsRequired = MutableStateFlow(false)

    /** True once ads may be requested. */
    val ready: StateFlow<Boolean> = _ready

    /** True when the user must be offered a way to change consent (EEA/UK). */
    val privacyOptionsRequired: StateFlow<Boolean> = _privacyOptionsRequired

    /**
     * Updates consent information and shows Google's consent form where the
     * law requires it, then starts the ads SDK. Call from onCreate.
     */
    fun gatherConsent(activity: Activity) {
        val consent = UserMessagingPlatform.getConsentInformation(activity)
        consent.requestConsentInfoUpdate(
            activity,
            ConsentRequestParameters.Builder().build(),
            {
                UserMessagingPlatform.loadAndShowConsentFormIfRequired(activity) {
                    update(activity, consent)
                }
            },
            { update(activity, consent) },
        )
        // Consent from an earlier launch allows ads right away.
        update(activity, consent)
    }

    /** Shows the consent form again, from the Settings screen. */
    fun showPrivacyOptions(activity: Activity) {
        UserMessagingPlatform.showPrivacyOptionsForm(activity) {
            update(activity, UserMessagingPlatform.getConsentInformation(activity))
        }
    }

    private fun update(activity: Activity, consent: ConsentInformation) {
        _privacyOptionsRequired.value =
            consent.privacyOptionsRequirementStatus == ConsentInformation.PrivacyOptionsRequirementStatus.REQUIRED
        if (!consent.canRequestAds() || started.getAndSet(true)) return
        val app = activity.applicationContext
        // Google recommends initialising off the main thread.
        Thread {
            MobileAds.initialize(app) { _ready.value = true }
        }.start()
    }
}

/** An adaptive banner; renders nothing until ads may be requested. */
@Composable
fun AdBanner(modifier: Modifier = Modifier) {
    val ready by Ads.ready.collectAsState()
    if (!ready) return
    val context = LocalContext.current
    val widthDp = LocalConfiguration.current.screenWidthDp
    val adView = remember(widthDp) {
        AdView(context).apply {
            adUnitId = BuildConfig.ADMOB_BANNER_ID
            setAdSize(AdSize.getCurrentOrientationAnchoredAdaptiveBannerAdSize(context, widthDp))
            loadAd(AdRequest.Builder().build())
        }
    }
    DisposableEffect(adView) {
        onDispose { adView.destroy() }
    }
    AndroidView(factory = { adView }, modifier = modifier.fillMaxWidth())
}
